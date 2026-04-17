package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunBase64(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{"EggQBCIEMgIKAA=="}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	got := stdout.String()
	if !strings.Contains(got, "seq=0") {
		t.Fatalf("output missing seq=0:\n%s", got)
	}
	if !strings.Contains(got, "type=PLAYBACK_EVENT") {
		t.Fatalf("output missing playback event type:\n%s", got)
	}
	if !strings.Contains(got, "states=[]") {
		t.Fatalf("output missing playback state summary:\n%s", got)
	}
}

func TestRunCaptureRange(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	// Capture fixtures under docs/captures/ are gitignored (they contain auth
	// cookies and session tokens), so contributors without a local capture run
	// cannot exercise this test. Skip cleanly instead of failing on a missing
	// path.
	path := filepath.Join("..", "..", "..", "docs", "captures", "interactive-audio", "webrtc-datachannel.jsonl")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skipf("fixture absent at %s; capture it via the interactive-audio capture tool to enable this test", path)
	}
	err := run([]string{"-capture", path, "-from", "42", "-to", "43"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("line count = %d, want 2\n%s", len(lines), stdout.String())
	}
	if !strings.Contains(lines[0], "seq=42") || !strings.Contains(lines[1], "seq=43") {
		t.Fatalf("unexpected output:\n%s", stdout.String())
	}
}

func TestParseFlagsRejectsMultipleSources(t *testing.T) {
	var stderr bytes.Buffer

	_, err := parseFlags([]string{"-base64", "EggQBCIEMgIKAA==", "-hex", "1210"}, &stderr)
	if err == nil {
		t.Fatal("parseFlags succeeded, want error")
	}
}
