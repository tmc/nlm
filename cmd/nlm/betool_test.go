package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// compactStr compacts JSON so pretty-printed output can be compared to a
// canonical single-line form.
func compactStr(t *testing.T, b []byte) string {
	t.Helper()
	var buf bytes.Buffer
	if err := json.Compact(&buf, b); err != nil {
		t.Fatalf("compact %q: %v", b, err)
	}
	return buf.String()
}

// runBetoolCapture runs a betool invocation, feeding stdinData on stdin and
// capturing stdout. It restores os.Stdin/os.Stdout before returning.
func runBetoolCapture(t *testing.T, args []string, stdinData string) (string, error) {
	t.Helper()

	origIn, origOut := os.Stdin, os.Stdout

	inR, inW, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stdin: %v", err)
	}
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stdout: %v", err)
	}
	os.Stdin, os.Stdout = inR, outW
	defer func() { os.Stdin, os.Stdout = origIn, origOut }()

	go func() {
		inW.WriteString(stdinData)
		inW.Close()
	}()

	done := make(chan string, 1)
	go func() {
		var b strings.Builder
		buf := make([]byte, 4096)
		for {
			n, err := outR.Read(buf)
			if n > 0 {
				b.Write(buf[:n])
			}
			if err != nil {
				break
			}
		}
		done <- b.String()
	}()

	// A leading "--json" selects the JSON output path (the real CLI honors the
	// global --json flag); strip it and pass it through as the flag.
	jsonOutput := false
	if len(args) > 0 && args[0] == "--json" {
		jsonOutput = true
		args = args[1:]
	}
	runErr := runBetool(args, jsonOutput)
	outW.Close()
	out := <-done
	outR.Close()
	inR.Close()
	return out, runErr
}

func TestBetoolRoundTripRequest(t *testing.T) {
	// JSON -> wire -> JSON reproduces the request, and the intermediate wire
	// body is a well-formed batchexecute form body.
	spec := `{"rpcs":[{"id":"wXbhsf","args":["My Notebook","😀"]}],"at":"TOK"}`

	body, err := runBetoolCapture(t, []string{"encode-request"}, spec)
	if err != nil {
		t.Fatalf("encode-request: %v", err)
	}
	body = strings.TrimSpace(body)
	if !strings.HasPrefix(body, "f.req=") || !strings.HasSuffix(body, "&") {
		t.Fatalf("bad wire body: %q", body)
	}
	// Space must be %20 (browser encodeURIComponent), never +.
	if strings.Contains(body, "My+Notebook") {
		t.Errorf("space encoded as + instead of %%20: %q", body)
	}

	jsonOut, err := runBetoolCapture(t, []string{"--json", "decode-request"}, body)
	if err != nil {
		t.Fatalf("decode-request: %v", err)
	}
	var got struct {
		RPCs []struct {
			ID   string          `json:"id"`
			Args json.RawMessage `json:"args"`
		} `json:"rpcs"`
		At string `json:"at"`
	}
	if err := json.Unmarshal([]byte(jsonOut), &got); err != nil {
		t.Fatalf("decode-request output not JSON: %v\n%s", err, jsonOut)
	}
	if len(got.RPCs) != 1 || got.RPCs[0].ID != "wXbhsf" || got.At != "TOK" {
		t.Fatalf("round-trip mismatch: %+v at=%q", got.RPCs, got.At)
	}
	if want := `["My Notebook","😀"]`; compactStr(t, got.RPCs[0].Args) != want {
		t.Errorf("args = %s, want %s", got.RPCs[0].Args, want)
	}
}

func TestBetoolRoundTripResponse(t *testing.T) {
	// A raw response body (with escaped inner payload) decodes to unescaped
	// JSON, and re-encoding then decoding reproduces the same JSON.
	raw := ")]}'\n\n" + `[["wrb.fr","CCqFvf","[[[\"nb-123\",\"My Notebook\"]]]",null,null,null,"generic"]]`

	jsonOut, err := runBetoolCapture(t, []string{"--json", "decode-response"}, raw)
	if err != nil {
		t.Fatalf("decode-response: %v", err)
	}
	if !strings.Contains(jsonOut, `"nb-123"`) || !strings.Contains(jsonOut, `"CCqFvf"`) {
		t.Fatalf("unexpected decode-response output:\n%s", jsonOut)
	}

	wire, err := runBetoolCapture(t, []string{"encode-response"}, jsonOut)
	if err != nil {
		t.Fatalf("encode-response: %v", err)
	}
	if !strings.HasPrefix(strings.TrimSpace(wire), ")]}'") {
		t.Fatalf("missing anti-hijack prefix: %q", wire)
	}

	jsonOut2, err := runBetoolCapture(t, []string{"--json", "decode-response"}, wire)
	if err != nil {
		t.Fatalf("decode-response (2): %v", err)
	}
	if strings.Join(strings.Fields(jsonOut), "") != strings.Join(strings.Fields(jsonOut2), "") {
		t.Errorf("response round-trip not stable:\n a=%s\n b=%s", jsonOut, jsonOut2)
	}
}

func TestBetoolFileInput(t *testing.T) {
	// betool reads its payload from a file argument as well as stdin.
	dir := t.TempDir()
	path := filepath.Join(dir, "req.json")
	if err := os.WriteFile(path, []byte(`{"rpcs":[{"id":"X","args":[1,2]}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := runBetoolCapture(t, []string{"encode-request", path}, "")
	if err != nil {
		t.Fatalf("encode-request from file: %v", err)
	}
	if !strings.HasPrefix(strings.TrimSpace(out), "f.req=") {
		t.Errorf("unexpected output: %q", out)
	}
}

func TestBetoolErrors(t *testing.T) {
	tests := []struct {
		name  string
		args  []string
		stdin string
		want  string
	}{
		{"no mode", nil, "", "missing mode"},
		{"unknown mode", []string{"nope"}, "", "unknown mode"},
		{"too many files", []string{"decode-request", "a", "b"}, "", "at most one input file"},
		{"bad request json", []string{"encode-request"}, "not json", "parse request JSON"},
		{"empty request body", []string{"decode-request"}, "", "decode request"},
		{"bad response json", []string{"encode-response"}, "not json", "parse response JSON"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := runBetoolCapture(t, tt.args, tt.stdin)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.want)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %q, want substring %q", err, tt.want)
			}
		})
	}
}

func TestBetoolProtoRequest(t *testing.T) {
	// A request whose args are fully modeled decodes into a typed proto and
	// verifies as lossless.
	spec := `{"rpcs":[{"id":"CCqFvf","args":["My Notebook","📓"]}]}`
	wire, err := runBetoolCapture(t, []string{"encode-request"}, spec)
	if err != nil {
		t.Fatalf("encode-request: %v", err)
	}
	out, err := runBetoolCapture(t, []string{"--json", "decode-request", "--verify"}, wire)
	if err != nil {
		t.Fatalf("decode-request --verify: %v", err)
	}
	var envs []struct {
		RPCID    string          `json:"rpc_id"`
		Method   string          `json:"method"`
		Type     string          `json:"type"`
		Message  json.RawMessage `json:"message"`
		Lossless *bool           `json:"roundtrip_lossless"`
	}
	if err := json.Unmarshal([]byte(out), &envs); err != nil {
		t.Fatalf("output not proto-envelope JSON: %v\n%s", err, out)
	}
	if len(envs) != 1 {
		t.Fatalf("got %d envelopes, want 1", len(envs))
	}
	e := envs[0]
	if e.RPCID != "CCqFvf" || e.Type != "notebooklm.v1alpha1.CreateProjectRequest" {
		t.Errorf("rpc_id/type = %q/%q", e.RPCID, e.Type)
	}
	if compactStr(t, e.Message) != `{"title":"My Notebook","emoji":"📓"}` {
		t.Errorf("message = %s", e.Message)
	}
	if e.Lossless == nil || !*e.Lossless {
		t.Errorf("expected lossless=true, got %v", e.Lossless)
	}
}

func TestBetoolProtoResponseFixture(t *testing.T) {
	// The list_notebooks fixture decodes into a typed response with named
	// fields; --verify surfaces the known unmodeled SourceMetadata field #4.
	raw, err := os.ReadFile("../../internal/batchexecute/testdata/list_notebooks.txt")
	if err != nil {
		t.Skipf("fixture unavailable: %v", err)
	}
	out, err := runBetoolCapture(t, []string{"--json", "decode-response", "--verify", "--rpc-id=wXbhsf"}, string(raw))
	if err != nil {
		t.Fatalf("decode-response --verify: %v", err)
	}
	if !strings.Contains(out, `"title": "nbname2"`) {
		t.Errorf("expected named field title=nbname2 in output:\n%s", out[:min(len(out), 400)])
	}
	if !strings.Contains(out, `"source_type": "SOURCE_TYPE_WEB_PAGE"`) {
		t.Errorf("expected enum SOURCE_TYPE_WEB_PAGE resolved")
	}
	if !strings.Contains(out, `"roundtrip_lossless": false`) {
		t.Errorf("expected lossless=false (fixture has an unmodeled field)")
	}

	// The default --verify view groups the four per-source findings into one
	// group whose displayed path stars only the varying source index, keeping
	// the SourceMetadata field-4 landmark ([2][3]). The full list is not
	// attached by default.
	var envs []struct {
		MissingCount  int `json:"missing_field_count"`
		MissingGroups []struct {
			Path   string `json:"path"`
			Kind   string `json:"kind"`
			Count  int    `json:"count"`
			Shapes int    `json:"shapes"`
		} `json:"missing_field_groups"`
		Missing []json.RawMessage `json:"missing_fields"`
	}
	if err := json.Unmarshal([]byte(out), &envs); err != nil {
		t.Fatalf("output not proto-envelope JSON: %v\n%s", err, out)
	}
	e := envs[0]
	if e.MissingCount != 4 {
		t.Errorf("missing_field_count = %d, want 4", e.MissingCount)
	}
	if len(e.MissingGroups) != 1 {
		t.Fatalf("got %d groups, want 1: %+v", len(e.MissingGroups), e.MissingGroups)
	}
	g := e.MissingGroups[0]
	if g.Path != "[0][0][1][*][2][3]" || g.Kind != "unmodeled" || g.Count != 4 || g.Shapes != 1 {
		t.Errorf("group = %+v, want path=[0][0][1][*][2][3] kind=unmodeled count=4 shapes=1", g)
	}
	if len(e.Missing) != 0 {
		t.Errorf("default --verify should not attach the full missing_fields list, got %d", len(e.Missing))
	}

	// --verify-all attaches the full unabridged list and still reports the count.
	allOut, err := runBetoolCapture(t, []string{"--json", "decode-response", "--verify-all", "--rpc-id=wXbhsf"}, string(raw))
	if err != nil {
		t.Fatalf("decode-response --verify-all: %v", err)
	}
	if err := json.Unmarshal([]byte(allOut), &envs); err != nil {
		t.Fatalf("--verify-all output not JSON: %v\n%s", err, allOut)
	}
	if got := len(envs[0].Missing); got != 4 {
		t.Errorf("--verify-all: got %d full findings, want 4", got)
	}
	if envs[0].MissingCount != 4 {
		t.Errorf("--verify-all: missing_field_count = %d, want 4 (always populated)", envs[0].MissingCount)
	}
}

// TestBetoolProtoConversationHistoryGrouping exercises grouping on a real
// GetConversationHistory capture (sanitized): the static ChatMessage type is
// misaligned with the wire turn shape, so all six turns are dropped as whole
// elements. They collapse into one group whose displayed path stars only the
// varying turn index, and the distinct user/assistant turn layouts surface as
// multiple shapes — the case single-shape list_notebooks does not cover.
func TestBetoolProtoConversationHistoryGrouping(t *testing.T) {
	raw, err := os.ReadFile("../../internal/batchexecute/testdata/conversation_history.txt")
	if err != nil {
		t.Skipf("fixture unavailable: %v", err)
	}
	out, err := runBetoolCapture(t, []string{"--json", "decode-response", "--verify", "--rpc-id=khqZz"}, string(raw))
	if err != nil {
		t.Fatalf("decode-response --verify: %v", err)
	}
	var envs []struct {
		Type          string `json:"type"`
		MissingCount  int    `json:"missing_field_count"`
		MissingGroups []struct {
			Path   string `json:"path"`
			Kind   string `json:"kind"`
			Count  int    `json:"count"`
			Shapes int    `json:"shapes"`
		} `json:"missing_field_groups"`
	}
	if err := json.Unmarshal([]byte(out), &envs); err != nil {
		t.Fatalf("output not proto-envelope JSON: %v\n%s", err, out)
	}
	e := envs[0]
	if e.Type != "notebooklm.v1alpha1.GetConversationHistoryResponse" {
		t.Errorf("type = %q", e.Type)
	}
	if len(e.MissingGroups) != 1 {
		t.Fatalf("got %d groups, want 1: %+v", len(e.MissingGroups), e.MissingGroups)
	}
	g := e.MissingGroups[0]
	if g.Path != "[0][*]" || g.Kind != "unmodeled" || g.Count != 6 {
		t.Errorf("group = %+v, want path=[0][*] kind=unmodeled count=6", g)
	}
	if g.Shapes < 2 {
		t.Errorf("shapes = %d, want >1 (user and assistant turns differ)", g.Shapes)
	}
}

func TestBetoolProtoErrors(t *testing.T) {
	// Build a raw request body carrying an unknown rpc_id, and a raw response
	// body carrying the ambiguous R7cb6c id.
	unknownReq, err := runBetoolCapture(t, []string{"encode-request"}, `{"rpcs":[{"id":"zzz","args":[]}]}`)
	if err != nil {
		t.Fatalf("encode-request: %v", err)
	}
	ambResp, err := runBetoolCapture(t, []string{"encode-response"}, `{"responses":[{"id":"R7cb6c","data":[]}]}`)
	if err != nil {
		t.Fatalf("encode-response: %v", err)
	}

	tests := []struct {
		name  string
		args  []string
		stdin string
		want  string
	}{
		{"unknown rpc_id", []string{"decode-request", "--proto"}, unknownReq, "no proto method bound"},
		{"ambiguous rpc_id", []string{"decode-response", "--proto"}, ambResp, "multiple methods"},
		{"proto on encode", []string{"encode-request", "--proto"}, `{"rpcs":[]}`, "apply only to decode"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := runBetoolCapture(t, tt.args, tt.stdin)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.want)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %q, want substring %q", err, tt.want)
			}
		})
	}
}
