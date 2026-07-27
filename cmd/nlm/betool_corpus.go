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
	"github.com/tmc/nlm/internal/rpcinfo"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type corpusAudit struct {
	Records     []corpusAuditRecord `json:"records"`
	Summary     corpusAuditSummary  `json:"summary"`
	HTTPRecords int                 `json:"-"`
}

type corpusAuditRecord struct {
	File             string               `json:"file"`
	Record           int                  `json:"record"`
	Side             string               `json:"side"`
	RPCID            string               `json:"rpc_id"`
	Method           string               `json:"method,omitempty"`
	MethodCandidates []string             `json:"method_candidates,omitempty"`
	Type             string               `json:"type,omitempty"`
	Status           string               `json:"status"`
	HTTPStatus       int                  `json:"http_status,omitempty"`
	MissingCount     int                  `json:"missing_field_count,omitempty"`
	MissingFields    []fieldDelta         `json:"missing_fields,omitempty"`
	Error            string               `json:"error,omitempty"`
	Evidence         json.RawMessage      `json:"evidence,omitempty"`
	RichContent      *richContentCoverage `json:"rich_content,omitempty"`
}

type richContentCoverage struct {
	RichDocuments            int            `json:"rich_documents"`
	SpanLayers               int            `json:"span_layers"`
	ChatAnswerDocuments      int            `json:"chat_answer_documents"`
	ContentSegmentDocuments  int            `json:"content_segment_documents"`
	Spans                    int            `json:"spans"`
	Content                  int            `json:"content"`
	Tables                   int            `json:"tables"`
	TableTypeCodes           map[int64]int  `json:"table_type_codes,omitempty"`
	CodeBlocks               int            `json:"code_blocks"`
	CodeBlockEmptyLanguage   int            `json:"code_block_empty_language"`
	CodeBlockLanguages       map[string]int `json:"code_block_languages,omitempty"`
	HiddenContent            int            `json:"hidden_content"`
	Separators               int            `json:"separators"`
	Leaves                   int            `json:"leaves"`
	Groups                   int            `json:"groups"`
	TableContainingCodeBlock int            `json:"table_containing_code_block"`
	HiddenContainingTable    int            `json:"hidden_containing_table"`
}

type corpusAuditSummary struct {
	Total               int                 `json:"total"`
	HTTPRecords         int                 `json:"http_records"`
	RPCPayloads         int                 `json:"rpc_payloads"`
	NonRPCRecords       int                 `json:"non_rpc_records"`
	BySide              map[string]int      `json:"by_side"`
	ByStatus            map[string]int      `json:"by_status"`
	RequestLossless     int                 `json:"request_lossless"`
	RequestValid        int                 `json:"request_valid"`
	ResponseLossless    int                 `json:"response_lossless"`
	ResponseValid       int                 `json:"response_valid"`
	UnexplainedFailures int                 `json:"unexplained_failures"`
	RichContent         richContentCoverage `json:"rich_content"`
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
			if rpcID := corpusStreamRPCID(entry.Request.URL); rpcID != "" {
				auditCorpusStream(audit, file, record, rpcID, entry)
				continue
			}
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

func corpusStreamRPCID(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	if strings.HasSuffix(u.Path, "/GenerateFreeFormStreamed") {
		return "laWbsf"
	}
	return ""
}

func auditCorpusStream(audit *corpusAudit, file string, record int, rpcID string, entry corpusTrafficEntry) {
	requestBody, err := base64.StdEncoding.DecodeString(entry.Request.PostData.Text)
	if err != nil {
		requestBody = []byte(entry.Request.PostData.Text)
	}
	requestWire, err := decodeWrbFRRequest(requestBody)
	if err != nil {
		appendCorpusFailures(audit, file, record, "request", []string{rpcID}, entry.Response.Status, "parser_failure", err)
	} else {
		audit.Records = append(audit.Records, auditCorpusWire(file, record, "request", rpcID, entry.Response.Status, requestWire))
	}

	responseBody := []byte(entry.Response.Content.Text)
	if strings.EqualFold(entry.Response.Content.Encoding, "base64") {
		decoded, err := base64.StdEncoding.DecodeString(entry.Response.Content.Text)
		if err != nil {
			appendCorpusFailures(audit, file, record, "response", []string{rpcID}, entry.Response.Status, "parser_failure", err)
			return
		}
		responseBody = decoded
	}
	if len(bytes.TrimSpace(responseBody)) == 0 {
		appendCorpusFailures(audit, file, record, "response", []string{rpcID}, entry.Response.Status, "transport_no_payload", fmt.Errorf("empty response body"))
		return
	}
	response, _, err := decodeWrbFRStream(responseBody, rpcID)
	if err != nil {
		status := "parser_failure"
		if entry.Response.Status != 200 {
			status = "transport_no_payload"
		}
		appendCorpusFailures(audit, file, record, "response", []string{rpcID}, entry.Response.Status, status, err)
		return
	}
	if first := response.Responses[0]; first.Status != 0 {
		audit.Records = append(audit.Records, corpusAuditRecord{
			File: file, Record: record, Side: "response", RPCID: rpcID,
			Status: "transport_no_payload", HTTPStatus: entry.Response.Status,
			Error: corpusStatusError(first.Status),
		})
		return
	}
	audit.Records = append(audit.Records, auditCorpusWire(file, record, "response", rpcID, entry.Response.Status, response.Responses[0].Data))
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
		// A frame that carries a gRPC status code instead of a payload has no
		// response message to model. Attempting to unmarshal the bare code
		// would either report it as a lossy field or, when the code happens to
		// fit field 1 of the response type, silently pass as lossless.
		if call.Status != 0 {
			audit.Records = append(audit.Records, corpusAuditRecord{
				File: file, Record: record, Side: "response", RPCID: call.ID,
				Status: "transport_no_payload", HTTPStatus: entry.Response.Status,
				Error: corpusStatusError(call.Status),
			})
			continue
		}
		audit.Records = append(audit.Records, auditCorpusWire(file, record, "response", call.ID, entry.Response.Status, call.Data))
	}
	for _, id := range missingCorpusRPCIDs(expected, seen) {
		appendCorpusFailures(audit, file, record, "response", []string{id}, entry.Response.Status, "transport_no_payload", fmt.Errorf("response envelope contains no payload for rpc_id"))
	}
}

// corpusStatusError describes a gRPC status code carried in place of a
// response payload, using the shared error dictionary so the wording matches
// what the client reports at runtime.
func corpusStatusError(status int) string {
	if code, ok := batchexecute.GetErrorCode(status); ok {
		return fmt.Sprintf("rpc failed with status %d (%s); frame carries no payload", status, code.Message)
	}
	return fmt.Sprintf("rpc failed with status %d; frame carries no payload", status)
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
		if err := beprotoUnmarshal(wire, msg); err != nil {
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
	out.RichContent = collectRichContent(best.msg)
	if len(best.deltas) == 0 {
		out.Status = "lossless"
	} else {
		out.Status = "lossy"
	}
	return out
}

func collectRichContent(msg proto.Message) *richContentCoverage {
	coverage := &richContentCoverage{}
	var visit func(protoreflect.Message)
	visit = func(message protoreflect.Message) {
		name := message.Descriptor().FullName()
		switch name {
		case "notebooklm.v1alpha1.RichDocument":
			coverage.RichDocuments++
		case "notebooklm.v1alpha1.SpanLayers":
			coverage.SpanLayers++
		case "notebooklm.v1alpha1.ChatAnswer":
			if field := message.Descriptor().Fields().ByNumber(5); message.Has(field) {
				coverage.ChatAnswerDocuments++
			}
		case "notebooklm.v1alpha1.ContentSegment":
			if field := message.Descriptor().Fields().ByNumber(5); message.Has(field) {
				coverage.ContentSegmentDocuments++
			}
		case "notebooklm.v1alpha1.Span":
			coverage.Spans++
			fields := message.Descriptor().Fields()
			if message.Has(fields.ByNumber(3)) {
				coverage.Content++
			}
			if field := fields.ByNumber(5); message.Has(field) {
				coverage.Tables++
				table := message.Get(field).Message()
				typeCode := table.Descriptor().Fields().ByNumber(1)
				if table.Has(typeCode) {
					if coverage.TableTypeCodes == nil {
						coverage.TableTypeCodes = make(map[int64]int)
					}
					coverage.TableTypeCodes[table.Get(typeCode).Int()]++
				}
				if messageHasSpanVariant(table, 7) {
					coverage.TableContainingCodeBlock++
				}
			}
			if field := fields.ByNumber(7); message.Has(field) {
				coverage.CodeBlocks++
				codeBlock := message.Get(field).Message()
				language := codeBlock.Get(codeBlock.Descriptor().Fields().ByNumber(2)).String()
				if language == "" {
					coverage.CodeBlockEmptyLanguage++
				} else {
					if coverage.CodeBlockLanguages == nil {
						coverage.CodeBlockLanguages = make(map[string]int)
					}
					coverage.CodeBlockLanguages[language]++
				}
			}
			if field := fields.ByNumber(9); message.Has(field) {
				coverage.HiddenContent++
				if messageHasSpanVariant(message.Get(field).Message(), 5) {
					coverage.HiddenContainingTable++
				}
			}
			if message.Has(fields.ByNumber(12)) {
				coverage.Separators++
			}
		case "notebooklm.v1alpha1.SpanContent":
			fields := message.Descriptor().Fields()
			if message.Has(fields.ByNumber(1)) {
				coverage.Leaves++
			}
			if message.Has(fields.ByNumber(2)) {
				coverage.Groups++
			}
		}
		message.Range(func(field protoreflect.FieldDescriptor, value protoreflect.Value) bool {
			if field.IsList() && field.Message() != nil {
				list := value.List()
				for i := 0; i < list.Len(); i++ {
					visit(list.Get(i).Message())
				}
			} else if field.Message() != nil {
				visit(value.Message())
			}
			return true
		})
	}
	visit(msg.ProtoReflect())
	if coverage.Spans == 0 {
		return nil
	}
	return coverage
}

func messageHasSpanVariant(message protoreflect.Message, fieldNumber protoreflect.FieldNumber) bool {
	if message.Descriptor().FullName() == "notebooklm.v1alpha1.Span" {
		if field := message.Descriptor().Fields().ByNumber(fieldNumber); field != nil && message.Has(field) {
			return true
		}
	}
	found := false
	message.Range(func(field protoreflect.FieldDescriptor, value protoreflect.Value) bool {
		if field.IsList() && field.Message() != nil {
			list := value.List()
			for i := 0; i < list.Len(); i++ {
				if messageHasSpanVariant(list.Get(i).Message(), fieldNumber) {
					found = true
					return false
				}
			}
		} else if field.Message() != nil && messageHasSpanVariant(value.Message(), fieldNumber) {
			found = true
			return false
		}
		return !found
	})
	return found
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
		if record.RichContent != nil {
			mergeRichContent(&summary.RichContent, record.RichContent)
		}
	}
	return summary
}

func mergeRichContent(dst *richContentCoverage, src *richContentCoverage) {
	dst.RichDocuments += src.RichDocuments
	dst.SpanLayers += src.SpanLayers
	dst.ChatAnswerDocuments += src.ChatAnswerDocuments
	dst.ContentSegmentDocuments += src.ContentSegmentDocuments
	dst.Spans += src.Spans
	dst.Content += src.Content
	dst.Tables += src.Tables
	dst.CodeBlocks += src.CodeBlocks
	dst.CodeBlockEmptyLanguage += src.CodeBlockEmptyLanguage
	dst.HiddenContent += src.HiddenContent
	dst.Separators += src.Separators
	dst.Leaves += src.Leaves
	dst.Groups += src.Groups
	dst.TableContainingCodeBlock += src.TableContainingCodeBlock
	dst.HiddenContainingTable += src.HiddenContainingTable
	if len(src.TableTypeCodes) > 0 {
		if dst.TableTypeCodes == nil {
			dst.TableTypeCodes = make(map[int64]int)
		}
		for value, count := range src.TableTypeCodes {
			dst.TableTypeCodes[value] += count
		}
	}
	if len(src.CodeBlockLanguages) > 0 {
		if dst.CodeBlockLanguages == nil {
			dst.CodeBlockLanguages = make(map[string]int)
		}
		for value, count := range src.CodeBlockLanguages {
			dst.CodeBlockLanguages[value] += count
		}
	}
}
