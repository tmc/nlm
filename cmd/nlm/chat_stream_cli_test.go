package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestInitialChatResponseWaiter(t *testing.T) {
	var status bytes.Buffer
	received, stop := startInitialChatResponseWaiter(&status, time.Millisecond)
	t.Cleanup(stop)

	time.Sleep(10 * time.Millisecond)
	received()
	stop()

	if got := status.String(); !strings.Contains(got, "waiting for initial NotebookLM response") {
		t.Fatalf("status = %q, want waiting notice", got)
	}
}

// TestPrintStreamFallback covers the dedup helper used when the streaming
// path errors out and we fall back to the non-streaming endpoint. The real
// bug it guards against: printing the full fallback on top of already-
// streamed bytes duplicated every completed section.
func TestPrintStreamFallback(t *testing.T) {
	tests := []struct {
		name     string
		streamed string
		full     string
		jsonl    bool
		want     string
	}{
		{
			name: "nothing streamed prints full response",
			full: "Hello.\nWorld.\n",
			want: "Hello.\nWorld.\n",
		},
		{
			name:     "prefix match prints only the suffix",
			streamed: "Section 1: intro.\nSection 2: ",
			full:     "Section 1: intro.\nSection 2: body.\nSection 3: end.\n",
			want:     "body.\nSection 3: end.\n",
		},
		{
			name:     "identical streamed and full prints nothing",
			streamed: "Complete.\n",
			full:     "Complete.\n",
		},
		{
			name:     "divergent fallback emits boundary and full response",
			streamed: "Section 1: intro.\nSection 2: ",
			full:     "Totally different answer.\n",
			want:     "\n--- streaming failed, re-rendering full response ---\nTotally different answer.\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var buf bytes.Buffer
			printStreamFallback(&buf, test.streamed, test.full, test.jsonl)
			if got := buf.String(); got != test.want {
				t.Fatalf("printStreamFallback out = %q, want %q", got, test.want)
			}
		})
	}
}

func TestPrintStreamFallbackJSONL(t *testing.T) {
	var buf bytes.Buffer
	printStreamFallback(&buf, "partial streamed text", "full fallback text", true)

	line := strings.TrimRight(buf.String(), "\n")
	if line == "" {
		t.Fatal("expected one JSONL event, got empty output")
	}
	if strings.Contains(line, "\n") {
		t.Fatalf("expected exactly one JSONL line, got %q", buf.String())
	}
	var event map[string]any
	if err := json.Unmarshal([]byte(line), &event); err != nil {
		t.Fatalf("fallback event is not valid JSON: %v (%q)", err, line)
	}
	if event["phase"] != "fallback" {
		t.Fatalf("phase = %v, want fallback", event["phase"])
	}
	if event["text"] != "full fallback text" {
		t.Fatalf("text = %v, want full fallback text", event["text"])
	}
}
