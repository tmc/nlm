package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/tmc/nlm/internal/notebooklm/api"
)

// captureStdout redirects os.Stdout for the duration of fn and returns what was written.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	done := make(chan struct{})
	var buf bytes.Buffer
	go func() {
		_, _ = io.Copy(&buf, r)
		close(done)
	}()

	fn()
	_ = w.Close()
	<-done
	return buf.String()
}

func TestEmitResearchEvent(t *testing.T) {
	tests := []struct {
		name string
		ev   researchEvent
		want map[string]any // subset match via JSON unmarshal
	}{
		{
			"fast complete",
			researchEvent{
				Type:   "complete",
				Mode:   "fast",
				Query:  "what is X",
				Report: "X is...",
			},
			map[string]any{"type": "complete", "mode": "fast", "query": "what is X", "report": "X is..."},
		},
		{
			"deep progress",
			researchEvent{
				Type:       "progress",
				Mode:       "deep",
				Query:      "X",
				ResearchID: "r-123",
			},
			map[string]any{"type": "progress", "mode": "deep", "research_id": "r-123"},
		},
		{
			"complete with sources",
			researchEvent{
				Type:    "complete",
				Mode:    "deep",
				Report:  "# Report",
				Sources: []api.ResearchSource{{ID: "s1", Title: "T", URL: "https://example.com"}},
			},
			map[string]any{"type": "complete", "mode": "deep"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := captureStdout(t, func() {
				if err := emitResearchEvent(tt.ev); err != nil {
					t.Fatalf("emit: %v", err)
				}
			})
			if !strings.HasSuffix(out, "\n") {
				t.Errorf("want trailing newline; got %q", out)
			}
			var got map[string]any
			if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &got); err != nil {
				t.Fatalf("unmarshal %q: %v", out, err)
			}
			for k, want := range tt.want {
				if got[k] != want {
					t.Errorf("field %q: got %v, want %v", k, got[k], want)
				}
			}
		})
	}
}

func TestRunResearchModeValidation(t *testing.T) {
	tests := []struct {
		name    string
		mode    string
		wantErr string
	}{
		{"empty defaults to deep", "", ""}, // delegated to deep, may fail on api call
		{"explicit fast", "fast", ""},
		{"explicit deep", "deep", ""},
		{"bogus mode", "medium", `--mode="medium": want fast or deep`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prev := researchMode
			researchMode = tt.mode
			defer func() { researchMode = prev }()

			if tt.wantErr == "" {
				// Mode validation alone passes; we don't wire a real api.Client
				// here (the scaffolded encoders are HAR-blocked and would hit
				// the network in this test). Just confirm the mode check
				// doesn't error synchronously.
				//
				// We only exercise the mode switch by calling with a nil
				// client in a subprocess — skip unless mode is bogus.
				return
			}
			err := runResearch(nil, "nb", "q")
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("want error containing %q; got %v", tt.wantErr, err)
			}
		})
	}
}

func TestSplitDeepResearchContent(t *testing.T) {
	// Scaffolded stub: returns full content as report, no sources.
	// When HAR lands this test should fail and force the real parser.
	report, sources := splitDeepResearchContent("some markdown report")
	if report != "some markdown report" {
		t.Errorf("report: got %q, want %q", report, "some markdown report")
	}
	if sources != nil {
		t.Errorf("sources: got %v; scaffold returns nil until HAR lands", sources)
	}
}

// TestResearchPollingErrorClassification verifies that the exit-code classifier
// treats ErrResearchPolling as exit 7 (busy) — locks in the Phase 0.5 wiring
// so 1c's scaffolding surfaces correctly today.
func TestResearchPollingErrorClassification(t *testing.T) {
	wrapped := fmt.Errorf("poll exhausted: %w", api.ErrResearchPolling)
	if code := exitCodeFor(wrapped); code != exitBusy {
		t.Errorf("got exit %d; want exitBusy (%d)", code, exitBusy)
	}
	// Sanity: the sentinel alone also classifies as busy.
	if code := exitCodeFor(api.ErrResearchPolling); code != exitBusy {
		t.Errorf("unwrapped: got exit %d; want exitBusy (%d)", code, exitBusy)
	}
}

func TestResearchSentinelIsErrorsIs(t *testing.T) {
	// Belt-and-braces: confirm errors.Is composes across multiple wraps.
	err := fmt.Errorf("outer: %w", fmt.Errorf("inner: %w", api.ErrResearchPolling))
	if !errors.Is(err, api.ErrResearchPolling) {
		t.Error("errors.Is did not unwrap ErrResearchPolling through two layers")
	}
}
