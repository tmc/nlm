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

	runErr := runBetool(args)
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

	jsonOut, err := runBetoolCapture(t, []string{"decode-request"}, body)
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

	jsonOut, err := runBetoolCapture(t, []string{"decode-response"}, raw)
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

	jsonOut2, err := runBetoolCapture(t, []string{"decode-response"}, wire)
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
