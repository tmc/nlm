package batchexecute

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestDecodeRequest(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		wantRPC  string
		wantArgs string
		wantAt   string
	}{
		{
			name:     "full form body",
			body:     `f.req=%5B%5B%5B%22wXbhsf%22%2C%22%5B%5D%22%2Cnull%2C%22generic%22%5D%5D%5D&at=ABC123&`,
			wantRPC:  "wXbhsf",
			wantArgs: "[]",
			wantAt:   "ABC123",
		},
		{
			name:     "bare envelope",
			body:     `[[["CCqFvf","[\"nb-123\"]",null,"generic"]]]`,
			wantRPC:  "CCqFvf",
			wantArgs: `["nb-123"]`,
			wantAt:   "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := DecodeRequest(tt.body)
			if err != nil {
				t.Fatalf("DecodeRequest: %v", err)
			}
			if len(req.RPCs) != 1 {
				t.Fatalf("got %d rpcs, want 1", len(req.RPCs))
			}
			if req.RPCs[0].ID != tt.wantRPC {
				t.Errorf("id = %q, want %q", req.RPCs[0].ID, tt.wantRPC)
			}
			if got := string(req.RPCs[0].Args); got != tt.wantArgs {
				t.Errorf("args = %q, want %q", got, tt.wantArgs)
			}
			if req.At != tt.wantAt {
				t.Errorf("at = %q, want %q", req.At, tt.wantAt)
			}
		})
	}
}

func TestRequestRoundTrip(t *testing.T) {
	// Build a request from JSON, encode it, decode it, and confirm we recover
	// the same structure — and that encode is byte-stable across a round trip.
	spec := `{"rpcs":[{"id":"wXbhsf","args":["title","emoji"],"index":"generic"}],"at":"tok-42"}`
	var req WireRequest
	if err := json.Unmarshal([]byte(spec), &req); err != nil {
		t.Fatalf("unmarshal spec: %v", err)
	}

	body, err := EncodeRequest(&req)
	if err != nil {
		t.Fatalf("EncodeRequest: %v", err)
	}
	if !strings.HasPrefix(body, "f.req=") || !strings.HasSuffix(body, "&") {
		t.Fatalf("unexpected body shape: %q", body)
	}

	got, err := DecodeRequest(body)
	if err != nil {
		t.Fatalf("DecodeRequest: %v", err)
	}
	if len(got.RPCs) != 1 || got.RPCs[0].ID != "wXbhsf" {
		t.Fatalf("round-trip rpc mismatch: %+v", got.RPCs)
	}
	if string(got.RPCs[0].Args) != `["title","emoji"]` {
		t.Errorf("round-trip args = %q", got.RPCs[0].Args)
	}
	if got.At != "tok-42" {
		t.Errorf("round-trip at = %q", got.At)
	}

	// Re-encoding the decoded request must reproduce the identical body.
	body2, err := EncodeRequest(got)
	if err != nil {
		t.Fatalf("EncodeRequest (2): %v", err)
	}
	if body != body2 {
		t.Errorf("encode not byte-stable:\n a=%q\n b=%q", body, body2)
	}
}

func TestWireDecodeResponse(t *testing.T) {
	raw := ")]}'\n\n" + `[["wrb.fr","CCqFvf","[[[\"nb-123\",\"My Notebook\"]]]",null,null,null,"generic"]]`
	resp, err := DecodeResponse(raw)
	if err != nil {
		t.Fatalf("DecodeResponse: %v", err)
	}
	if len(resp.Responses) != 1 {
		t.Fatalf("got %d responses, want 1", len(resp.Responses))
	}
	r := resp.Responses[0]
	if r.ID != "CCqFvf" {
		t.Errorf("id = %q, want CCqFvf", r.ID)
	}
	// Data must be unescaped, directly-usable JSON.
	if !json.Valid(r.Data) {
		t.Fatalf("data is not valid JSON: %q", r.Data)
	}
	if want := `[[["nb-123","My Notebook"]]]`; string(r.Data) != want {
		t.Errorf("data = %q, want %q", r.Data, want)
	}
}

// TestDecodeResponseStatusFrame covers frames that carry a gRPC canonical
// status code at position 5 because position 2 has no payload. batchexecute
// tunnels RPC failures in-band, so these arrive with HTTP 200 and must not be
// mistaken for a response message: the bare code either looks like a lossy
// field or, when it happens to fit field 1 of the response type, passes as a
// successful decode. The status codes below are the ones observed on the wire.
func TestDecodeResponseStatusFrame(t *testing.T) {
	tests := []struct {
		name   string
		rpcID  string
		frame  string
		status int
	}{
		{name: "invalid argument", rpcID: "R7cb6c", frame: `[3]`, status: 3},
		{name: "not found", rpcID: "rtY7md", frame: `[5]`, status: 5},
		{name: "unimplemented", rpcID: "yyryJe", frame: `[12]`, status: 12},
		{name: "internal", rpcID: "Rytqqe", frame: `[13]`, status: 13},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := ")]}'\n\n" + fmt.Sprintf(`[["wrb.fr",%q,null,null,null,%s,"generic"]]`, tt.rpcID, tt.frame)
			resp, err := DecodeResponse(raw)
			if err != nil {
				t.Fatalf("DecodeResponse: %v", err)
			}
			if len(resp.Responses) != 1 {
				t.Fatalf("got %d responses, want 1", len(resp.Responses))
			}
			if got := resp.Responses[0].Status; got != tt.status {
				t.Errorf("status = %d, want %d", got, tt.status)
			}
		})
	}
}

// TestDecodeResponsePayloadHasNoStatus pins the other half of the rule: a frame
// carrying a real payload at position 2 reports status 0, so a successful
// response is never suppressed as a failure.
func TestDecodeResponsePayloadHasNoStatus(t *testing.T) {
	raw := ")]}'\n\n" + `[["wrb.fr","CCqFvf","[[[\"nb-123\",\"My Notebook\"]]]",null,null,null,"generic"]]`
	resp, err := DecodeResponse(raw)
	if err != nil {
		t.Fatalf("DecodeResponse: %v", err)
	}
	if got := resp.Responses[0].Status; got != 0 {
		t.Errorf("status = %d, want 0 for a frame with a payload", got)
	}
}

func TestResponseRoundTrip(t *testing.T) {
	spec := `{"responses":[{"id":"CCqFvf","index":0,"data":[[["nb-123","My Notebook"]]]}]}`
	var resp WireResponse
	if err := json.Unmarshal([]byte(spec), &resp); err != nil {
		t.Fatalf("unmarshal spec: %v", err)
	}

	raw, err := EncodeResponse(&resp)
	if err != nil {
		t.Fatalf("EncodeResponse: %v", err)
	}
	if !strings.HasPrefix(raw, ")]}'") {
		t.Fatalf("missing anti-hijack prefix: %q", raw)
	}

	got, err := DecodeResponse(raw)
	if err != nil {
		t.Fatalf("DecodeResponse: %v", err)
	}
	if len(got.Responses) != 1 || got.Responses[0].ID != "CCqFvf" {
		t.Fatalf("round-trip response mismatch: %+v", got.Responses)
	}
	if string(got.Responses[0].Data) != `[[["nb-123","My Notebook"]]]` {
		t.Errorf("round-trip data = %q", got.Responses[0].Data)
	}
}

// TestDecodeResponseChunkedRealWorld reproduces the three conditions seen in a
// real NotebookLM khqZz (note) response that broke the naive decoder:
//
//  1. the ")]}'" anti-hijack marker split across lines (")\n]\n}'\n\n");
//  2. length-prefixed chunked framing with trailing bookkeeping chunks; and
//  3. a literal newline inside a JSON string value (markdown body text).
func TestDecodeResponseChunkedRealWorld(t *testing.T) {
	// The inner note payload carries a body string with a real newline. On the
	// wire the payload is a JSON-encoded string, so quotes are backslash-escaped
	// but the newline is left literal.
	inner := "[[[\"note-1\",[[\"line one\nline two\"]]]]]"
	innerEscaped := strings.ReplaceAll(inner, `"`, `\"`)
	row := `["wrb.fr","khqZz","` + innerEscaped + `",null,null,null,"generic"]`
	dataChunk := "[" + row + "]"
	// A trailing bookkeeping chunk, as real responses append (e.g. "e"/af.httprm).
	tailChunk := `[["e",4,null,null,99]]`

	// Split prefix, then length-prefixed chunks.
	raw := ")\n]\n}'\n\n" +
		itoa(len(dataChunk)) + "\n" + dataChunk + "\n" +
		itoa(len(tailChunk)) + "\n" + tailChunk

	resp, err := DecodeResponse(raw)
	if err != nil {
		t.Fatalf("DecodeResponse: %v", err)
	}
	if len(resp.Responses) != 1 {
		t.Fatalf("got %d responses, want 1 (khqZz); tail chunk should not surface as a response", len(resp.Responses))
	}
	r := resp.Responses[0]
	if r.ID != "khqZz" {
		t.Errorf("id = %q, want khqZz", r.ID)
	}
	// Data must be fully-parsed JSON (not a re-escaped string), and the literal
	// newline inside the body must be preserved as a valid \n escape.
	if !json.Valid(r.Data) {
		t.Fatalf("data is not valid JSON: %s", r.Data)
	}
	var parsed [][][]any
	if err := json.Unmarshal(r.Data, &parsed); err != nil {
		t.Fatalf("data does not parse as the note structure: %v\n%s", err, r.Data)
	}
	body, _ := parsed[0][0][1].([]any)[0].([]any)[0].(string)
	if body != "line one\nline two" {
		t.Errorf("body text = %q, want %q", body, "line one\nline two")
	}
}

// itoa is a tiny helper so the test fixtures read clearly.
func itoa(n int) string {
	return fmt.Sprintf("%d", n)
}

func TestDecodeRequestErrors(t *testing.T) {
	for _, body := range []string{"", "at=x&", "not a valid body=%ZZ"} {
		if _, err := DecodeRequest(body); err == nil {
			t.Errorf("DecodeRequest(%q) = nil error, want error", body)
		}
	}
}
