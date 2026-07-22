package batchexecute

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// The batchexecute wire protocol packs RPC calls into a URL-encoded form body
// and returns responses as a length-prefixed or JSON-array envelope. The types
// and functions in this file translate between those raw wire payloads and a
// structured, human-readable JSON representation, in both directions, without
// performing any network I/O. They are the codec that the "nlm betool"
// developer command is built on.

// WireRequest is the structured form of an outgoing batchexecute request.
// It is the JSON representation of the "f.req" form field plus the "at" token.
type WireRequest struct {
	// RPCs are the individual calls packed into the request envelope.
	RPCs []WireRPC `json:"rpcs"`
	// At is the "at" (XSRF) token that accompanies the request body. It is
	// preserved so that DecodeRequest→EncodeRequest round-trips reproduce the
	// original form body; it is omitted from the JSON when empty.
	At string `json:"at,omitempty"`
}

// WireRPC is one call within a batchexecute request envelope.
//
// On the wire each call is [id, args, null, index], where args is itself a
// JSON string. Here Args is decoded into live JSON so the payload reads
// naturally; Index is "generic" for a single-call request or the numeric
// position (as a string) within a multi-call batch.
type WireRPC struct {
	ID    string          `json:"id"`
	Args  json.RawMessage `json:"args"`
	Index string          `json:"index,omitempty"`
}

// WireResponse is the structured form of a batchexecute response body: the
// list of decoded RPC responses carried by the envelope.
type WireResponse struct {
	Responses []WireRPCResponse `json:"responses"`
}

// WireRPCResponse is one decoded response within a batchexecute response body.
// Data is the inner payload with all wire-level string escaping removed, so it
// is directly usable JSON. Error is set only for error envelopes.
type WireRPCResponse struct {
	ID    string          `json:"id"`
	Index int             `json:"index"`
	Data  json.RawMessage `json:"data,omitempty"`
	Error string          `json:"error,omitempty"`
}

// DecodeRequest parses a raw batchexecute request form body — the verbatim
// "f.req=...&at=...&" POST body, or a bare URL-encoded f.req envelope — into a
// WireRequest. It reverses the encoding performed by EncodeRequest and by the
// client's Execute method.
func DecodeRequest(body string) (*WireRequest, error) {
	freq, at, err := splitRequestBody(body)
	if err != nil {
		return nil, err
	}

	// The f.req envelope is [[ [id, argsJSON, null, index], ... ]].
	var envelope [][]any
	if err := json.Unmarshal([]byte(freq), &envelope); err != nil {
		// Some callers pass the inner array directly ([ [id,...], ... ]).
		var inner []any
		if err2 := json.Unmarshal([]byte(freq), &inner); err2 != nil {
			return nil, fmt.Errorf("parse f.req envelope: %w", err)
		}
		envelope = [][]any{inner}
	}
	if len(envelope) == 0 {
		return nil, fmt.Errorf("empty f.req envelope")
	}

	req := &WireRequest{At: at}
	for _, call := range envelope[0] {
		fields, ok := call.([]any)
		if !ok || len(fields) < 2 {
			return nil, fmt.Errorf("malformed rpc call in envelope")
		}
		id, _ := fields[0].(string)
		if id == "" {
			return nil, fmt.Errorf("rpc call missing id")
		}
		rpc := WireRPC{ID: id}

		// fields[1] is the args, carried as a JSON string on the wire.
		switch v := fields[1].(type) {
		case string:
			if v == "" {
				rpc.Args = json.RawMessage("null")
			} else if json.Valid([]byte(v)) {
				rpc.Args = json.RawMessage(v)
			} else {
				return nil, fmt.Errorf("rpc %s: args is not valid JSON: %q", id, v)
			}
		case nil:
			rpc.Args = json.RawMessage("null")
		default:
			b, err := json.Marshal(v)
			if err != nil {
				return nil, fmt.Errorf("rpc %s: re-encode args: %w", id, err)
			}
			rpc.Args = json.RawMessage(b)
		}

		if len(fields) >= 4 {
			if idx, ok := fields[3].(string); ok {
				rpc.Index = idx
			}
		}
		req.RPCs = append(req.RPCs, rpc)
	}
	return req, nil
}

// EncodeRequest renders a WireRequest into the exact form body that
// batchexecute expects: "f.req=<url-encoded envelope>&at=<url-encoded token>&".
// It is the inverse of DecodeRequest.
func EncodeRequest(req *WireRequest) (string, error) {
	if req == nil || len(req.RPCs) == 0 {
		return "", fmt.Errorf("request has no rpcs")
	}

	envelope := make([]any, 0, len(req.RPCs))
	for _, rpc := range req.RPCs {
		if rpc.ID == "" {
			return "", fmt.Errorf("rpc call missing id")
		}
		// Args is stored as a JSON string on the wire. Compact it so that any
		// indentation from a pretty-printed WireRequest does not leak into the
		// embedded payload.
		argsField := "null"
		if len(rpc.Args) > 0 && string(rpc.Args) != "null" {
			compact, err := compactJSON(rpc.Args)
			if err != nil {
				return "", fmt.Errorf("rpc %s: args is not valid JSON: %w", rpc.ID, err)
			}
			argsField = compact
		}
		index := rpc.Index
		if index == "" {
			index = "generic"
		}
		envelope = append(envelope, []any{
			rpc.ID,
			argsField,
			nil,
			index,
		})
	}

	reqBody, err := json.Marshal([]any{envelope})
	if err != nil {
		return "", fmt.Errorf("marshal f.req envelope: %w", err)
	}
	// Match the browser's exact body layout: f.req first, then at, trailing &.
	return fmt.Sprintf("f.req=%s&at=%s&",
		queryEscapeWire(string(reqBody)),
		queryEscapeWire(req.At)), nil
}

// compactJSON removes insignificant whitespace from valid JSON. It returns an
// error if the input is not valid JSON.
func compactJSON(b json.RawMessage) (string, error) {
	var buf bytes.Buffer
	if err := json.Compact(&buf, b); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// queryEscapeWire percent-encodes s the way the browser's encodeURIComponent
// does, which is what batchexecute request bodies use. It differs from
// url.QueryEscape in one important way: a space becomes "%20", not "+".
func queryEscapeWire(s string) string {
	return strings.ReplaceAll(url.QueryEscape(s), "+", "%20")
}

// DecodeResponse parses a raw batchexecute response body — with or without the
// ")]}'" anti-JSON-hijacking prefix, in either the length-prefixed chunked
// format or the plain JSON-array format — into a WireResponse. All wire-level
// string escaping on the inner payloads is removed so Data is usable JSON.
//
// The parsing is delegated to the shared response decoder, which handles the
// prefix layouts, chunk framing, control-character sanitization, and payload
// normalization that real NotebookLM responses require.
func DecodeResponse(body string) (*WireResponse, error) {
	responses, err := decodeResponse(body)
	if err != nil {
		return nil, err
	}
	out := &WireResponse{}
	for _, r := range responses {
		out.Responses = append(out.Responses, WireRPCResponse{
			ID:    r.ID,
			Index: r.Index,
			Data:  r.Data,
			Error: r.Error,
		})
	}
	return out, nil
}

// normalizeResponseData turns a response payload into directly-usable JSON.
// When the inner payload contains literal control characters inside string
// values, the unescaper cannot validate it and returns it wrapped as a JSON
// string; this unwraps that case and sanitizes the control characters so the
// result is the parsed structure rather than an escaped blob. Data that is
// already valid JSON is returned unchanged.
func normalizeResponseData(data json.RawMessage) json.RawMessage {
	if len(data) == 0 {
		return data
	}
	// If Data is a JSON string, it is a payload that failed validation
	// upstream. Unwrap it, sanitize embedded control characters, and use it if
	// that yields valid JSON.
	var inner string
	if json.Unmarshal(data, &inner) == nil {
		sanitized := sanitizeJSONControlChars(inner)
		if json.Valid([]byte(sanitized)) {
			return json.RawMessage(sanitized)
		}
	}
	return data
}

// EncodeResponse renders a WireResponse back into a raw batchexecute response
// body in the JSON-array format, including the ")]}'" prefix. Decoding the
// result with DecodeResponse reproduces the input.
func EncodeResponse(resp *WireResponse) (string, error) {
	if resp == nil {
		return "", fmt.Errorf("response is nil")
	}
	envelope := make([]any, 0, len(resp.Responses))
	for _, r := range resp.Responses {
		// Reconstruct ["wrb.fr", id, dataJSONString, null, null, null, index].
		var dataField any
		if len(r.Data) > 0 {
			compact, err := compactJSON(r.Data)
			if err != nil {
				return "", fmt.Errorf("rpc %s: data is not valid JSON: %w", r.ID, err)
			}
			dataField = compact
		}
		index := "generic"
		if r.Index != 0 {
			index = fmt.Sprintf("%d", r.Index)
		}
		envelope = append(envelope, []any{
			"wrb.fr",
			r.ID,
			dataField,
			nil,
			nil,
			nil,
			index,
		})
	}
	body, err := json.Marshal(envelope)
	if err != nil {
		return "", fmt.Errorf("marshal response envelope: %w", err)
	}
	return ")]}'\n\n" + string(body), nil
}

// stripAntiHijackPrefix removes the ")]}'" anti-JSON-hijacking marker that
// batchexecute prepends to response bodies. Servers emit it in several layouts
// — contiguous ")]}'", or with each character on its own line (")\n]\n}'\n") —
// so this normalizes any leading run of the marker's four characters together
// with surrounding whitespace before the real payload begins. If no marker is
// present the body is returned with only leading whitespace trimmed, leaving
// the payload (including a leading digit for chunked bodies) intact.
func stripAntiHijackPrefix(body string) string {
	i := 0
	for i < len(body) {
		switch body[i] {
		case ')', ']', '}', '\'', ' ', '\t', '\r', '\n':
			i++
		default:
			// Only treat the leading run as a prefix if it actually contained
			// the marker's opening ")" — otherwise leave the body untouched so
			// that, e.g., a payload starting with "[" is not mistaken for one.
			if strings.ContainsRune(body[:i], ')') {
				return body[i:]
			}
			return strings.TrimLeft(body, " \t\r\n")
		}
	}
	return strings.TrimLeft(body, " \t\r\n")
}

// splitRequestBody extracts the f.req and at fields from a raw request body.
// The body may be the full "f.req=...&at=...&" POST form, or just a bare
// (optionally URL-encoded) f.req envelope with no key=value structure.
func splitRequestBody(body string) (freq, at string, err error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return "", "", fmt.Errorf("empty request body")
	}

	// A bare envelope starts with '[' and contains no form fields.
	if strings.HasPrefix(body, "[") {
		return body, "", nil
	}

	values, perr := url.ParseQuery(body)
	if perr != nil {
		return "", "", fmt.Errorf("parse request body: %w", perr)
	}
	freq = values.Get("f.req")
	if freq == "" {
		return "", "", fmt.Errorf("request body has no f.req field")
	}
	return freq, values.Get("at"), nil
}
