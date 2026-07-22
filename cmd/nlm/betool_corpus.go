package main

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"

	"github.com/tmc/nlm/internal/batchexecute"
	"github.com/tmc/nlm/internal/beprotojson"
	"github.com/tmc/nlm/internal/rpcinfo"
	"google.golang.org/protobuf/proto"
)

type corpusAudit struct {
	Records     []corpusAuditRecord `json:"records"`
	Summary     corpusAuditSummary  `json:"summary"`
	HTTPRecords int                 `json:"-"`
}

type corpusAuditRecord struct {
	File             string          `json:"file"`
	Record           int             `json:"record"`
	Side             string          `json:"side"`
	RPCID            string          `json:"rpc_id"`
	Method           string          `json:"method,omitempty"`
	MethodCandidates []string        `json:"method_candidates,omitempty"`
	Type             string          `json:"type,omitempty"`
	Status           string          `json:"status"`
	HTTPStatus       int             `json:"http_status,omitempty"`
	MissingCount     int             `json:"missing_field_count,omitempty"`
	MissingFields    []fieldDelta    `json:"missing_fields,omitempty"`
	Error            string          `json:"error,omitempty"`
	Evidence         json.RawMessage `json:"evidence,omitempty"`
}

type corpusAuditSummary struct {
	Total               int            `json:"total"`
	HTTPRecords         int            `json:"http_records"`
	RPCPayloads         int            `json:"rpc_payloads"`
	NonRPCRecords       int            `json:"non_rpc_records"`
	BySide              map[string]int `json:"by_side"`
	ByStatus            map[string]int `json:"by_status"`
	RequestLossless     int            `json:"request_lossless"`
	RequestValid        int            `json:"request_valid"`
	ResponseLossless    int            `json:"response_lossless"`
	ResponseValid       int            `json:"response_valid"`
	UnexplainedFailures int            `json:"unexplained_failures"`
}

type corpusTrafficEntry struct {
	Request struct {
		Method   string `json:"method"`
		URL      string `json:"url"`
		PostData struct {
			Text string `json:"text"`
		} `json:"postData"`
	} `json:"request"`
	Response struct {
		Status  int `json:"status"`
		Content struct {
			Text     string `json:"text"`
			Encoding string `json:"encoding"`
		} `json:"content"`
	} `json:"response"`
}

func betoolAuditCorpus(opts betoolOptions) error {
	audit := corpusAudit{}
	for _, file := range opts.files {
		if err := auditCorpusFile(&audit, file); err != nil {
			return err
		}
	}
	audit.Summary = summarizeCorpusAudit(audit.Records, audit.HTTPRecords)
	if opts.asJSON {
		return writeJSON(audit)
	}
	for _, record := range audit.Records {
		fmt.Fprintf(os.Stdout, "%s:%d\t%s\t%s\t%s", record.File, record.Record, record.Side, record.RPCID, record.Status)
		if record.Method != "" {
			fmt.Fprintf(os.Stdout, "\t%s", record.Method)
		}
		if record.Error != "" {
			fmt.Fprintf(os.Stdout, "\t%s", record.Error)
		}
		fmt.Fprintln(os.Stdout)
	}
	return nil
}

func auditCorpusFile(audit *corpusAudit, file string) error {
	data, err := os.ReadFile(file)
	if err != nil {
		return fmt.Errorf("audit-corpus: read %s: %w", file, err)
	}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 64*1024), 64*1024*1024)
	for record := 1; scanner.Scan(); record++ {
		audit.HTTPRecords++
		var entry corpusTrafficEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			audit.Records = append(audit.Records, corpusAuditRecord{
				File: file, Record: record, Side: "record", Status: "parser_failure", Error: err.Error(),
			})
			continue
		}
		ids := corpusRPCIDs(entry.Request.URL)
		if len(ids) == 0 {
			evidence, _ := json.Marshal(struct {
				Method string `json:"method"`
				Host   string `json:"host"`
				Path   string `json:"path"`
			}{
				Method: entry.Request.Method,
				Host:   corpusURLHost(entry.Request.URL),
				Path:   corpusURLPath(entry.Request.URL),
			})
			audit.Records = append(audit.Records, corpusAuditRecord{
				File: file, Record: record, Side: "record", Status: "non_rpc_http",
				HTTPStatus: entry.Response.Status, Error: "request URL contains no rpcids parameter",
				Evidence: evidence,
			})
			continue
		}
		auditCorpusRequest(audit, file, record, ids, entry)
		auditCorpusResponse(audit, file, record, ids, entry)
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("audit-corpus: scan %s: %w", file, err)
	}
	return nil
}

func corpusURLHost(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return u.Host
}

func corpusURLPath(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return u.Path
}

func corpusRPCIDs(rawURL string) []string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil
	}
	value := u.Query().Get("rpcids")
	if value == "" {
		return nil
	}
	var ids []string
	for _, id := range strings.Split(value, ",") {
		if id = strings.TrimSpace(id); id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func auditCorpusRequest(audit *corpusAudit, file string, record int, expected []string, entry corpusTrafficEntry) {
	body, err := base64.StdEncoding.DecodeString(entry.Request.PostData.Text)
	if err != nil {
		body = []byte(entry.Request.PostData.Text)
	}
	req, err := batchexecute.DecodeRequest(string(body))
	if err != nil {
		appendCorpusFailures(audit, file, record, "request", expected, entry.Response.Status, "parser_failure", err)
		return
	}
	if len(req.RPCs) == 0 {
		appendCorpusFailures(audit, file, record, "request", expected, entry.Response.Status, "parser_failure", fmt.Errorf("decoded request contains no RPCs"))
		return
	}
	seen := make(map[string]bool, len(req.RPCs))
	for _, call := range req.RPCs {
		seen[call.ID] = true
		audit.Records = append(audit.Records, auditCorpusWire(file, record, "request", call.ID, entry.Response.Status, call.Args))
	}
	for _, id := range missingCorpusRPCIDs(expected, seen) {
		appendCorpusFailures(audit, file, record, "request", []string{id}, entry.Response.Status, "parser_failure", fmt.Errorf("request envelope contains no payload for rpc_id"))
	}
}

func auditCorpusResponse(audit *corpusAudit, file string, record int, expected []string, entry corpusTrafficEntry) {
	body := []byte(entry.Response.Content.Text)
	if strings.EqualFold(entry.Response.Content.Encoding, "base64") {
		decoded, err := base64.StdEncoding.DecodeString(entry.Response.Content.Text)
		if err != nil {
			appendCorpusFailures(audit, file, record, "response", expected, entry.Response.Status, "parser_failure", err)
			return
		}
		body = decoded
	}
	if len(bytes.TrimSpace(body)) == 0 {
		appendCorpusFailures(audit, file, record, "response", expected, entry.Response.Status, "transport_no_payload", fmt.Errorf("empty response body"))
		return
	}
	resp, err := batchexecute.DecodeResponse(string(body))
	if err != nil {
		status := "parser_failure"
		if entry.Response.Status != 200 {
			status = "transport_no_payload"
		}
		appendCorpusFailures(audit, file, record, "response", expected, entry.Response.Status, status, err)
		return
	}
	if len(resp.Responses) == 0 {
		appendCorpusFailures(audit, file, record, "response", expected, entry.Response.Status, "transport_no_payload", fmt.Errorf("decoded response contains no RPC payloads"))
		return
	}
	seen := make(map[string]bool, len(resp.Responses))
	for _, call := range resp.Responses {
		seen[call.ID] = true
		if call.Error != "" && len(call.Data) == 0 {
			audit.Records = append(audit.Records, corpusAuditRecord{
				File: file, Record: record, Side: "response", RPCID: call.ID,
				Status: "transport_no_payload", HTTPStatus: entry.Response.Status, Error: call.Error,
			})
			continue
		}
		audit.Records = append(audit.Records, auditCorpusWire(file, record, "response", call.ID, entry.Response.Status, call.Data))
	}
	for _, id := range missingCorpusRPCIDs(expected, seen) {
		appendCorpusFailures(audit, file, record, "response", []string{id}, entry.Response.Status, "transport_no_payload", fmt.Errorf("response envelope contains no payload for rpc_id"))
	}
}

func missingCorpusRPCIDs(expected []string, seen map[string]bool) []string {
	var missing []string
	for _, id := range expected {
		if !seen[id] {
			missing = append(missing, id)
		}
	}
	return missing
}

func appendCorpusFailures(audit *corpusAudit, file string, record int, side string, ids []string, httpStatus int, status string, err error) {
	if len(ids) == 0 {
		ids = []string{""}
	}
	for _, id := range ids {
		rec := corpusAuditRecord{
			File: file, Record: record, Side: side, RPCID: id, Status: status, HTTPStatus: httpStatus,
		}
		if err != nil {
			rec.Error = err.Error()
		}
		audit.Records = append(audit.Records, rec)
	}
}

func auditCorpusWire(file string, record int, side, rpcID string, httpStatus int, wire json.RawMessage) corpusAuditRecord {
	out := corpusAuditRecord{
		File: file, Record: record, Side: side, RPCID: rpcID, HTTPStatus: httpStatus,
	}
	methods, err := rpcinfo.LookupAll(rpcID)
	if err != nil {
		out.Status = "parser_failure"
		out.Error = err.Error()
		return out
	}
	type candidate struct {
		method rpcinfo.Method
		msg    proto.Message
		deltas []fieldDelta
		err    error
	}
	candidates := make([]candidate, 0, len(methods))
	for _, method := range methods {
		var msg proto.Message
		if side == "request" {
			msg = method.NewRequest()
		} else {
			msg = method.NewResponse()
		}
		candidate := candidate{method: method, msg: msg}
		if err := beprotojson.Unmarshal(wire, msg); err != nil {
			candidate.err = err
		} else {
			candidate.deltas, candidate.err = diffWireAgainstProto(wire, msg)
		}
		candidates = append(candidates, candidate)
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].err != nil {
			return false
		}
		if candidates[j].err != nil {
			return true
		}
		return len(candidates[i].deltas) < len(candidates[j].deltas)
	})
	best := candidates[0]
	for _, candidate := range candidates {
		if candidate.err == nil && len(candidate.deltas) == len(best.deltas) {
			out.MethodCandidates = append(out.MethodCandidates, candidate.method.FullName())
		}
	}
	if best.err != nil {
		out.Status = "parser_failure"
		out.Error = best.err.Error()
		return out
	}
	out.Method = best.method.FullName()
	if side == "request" {
		out.Type = string(best.method.Request.Descriptor().FullName())
	} else {
		out.Type = string(best.method.Response.Descriptor().FullName())
	}
	out.MissingCount = len(best.deltas)
	out.MissingFields = best.deltas
	if len(best.deltas) == 0 {
		out.Status = "lossless"
	} else {
		out.Status = "lossy"
	}
	return out
}

func summarizeCorpusAudit(records []corpusAuditRecord, httpRecords int) corpusAuditSummary {
	summary := corpusAuditSummary{
		HTTPRecords: httpRecords,
		BySide:      make(map[string]int),
		ByStatus:    make(map[string]int),
	}
	for _, record := range records {
		summary.Total++
		summary.BySide[record.Side]++
		summary.ByStatus[record.Status]++
		if record.Side == "record" {
			if record.Status == "non_rpc_http" {
				summary.NonRPCRecords++
			}
		} else {
			summary.RPCPayloads++
		}
		switch record.Side {
		case "request":
			if record.Status == "lossless" {
				summary.RequestLossless++
			}
			if record.Status == "lossless" || record.Status == "lossy" {
				summary.RequestValid++
			}
		case "response":
			if record.Status == "lossless" {
				summary.ResponseLossless++
			}
			if record.Status == "lossless" || record.Status == "lossy" {
				summary.ResponseValid++
			}
		}
		if record.Status == "parser_failure" {
			summary.UnexplainedFailures++
		}
	}
	return summary
}
