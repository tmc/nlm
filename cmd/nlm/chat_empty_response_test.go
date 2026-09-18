package main

import (
	"fmt"
	"strings"
	"testing"
)

func TestLongestIdenticalRun(t *testing.T) {
	tests := []struct {
		name   string
		prompt string
		want   int
	}{
		{"empty", "", 0},
		{"one line", "just one", 1},
		{"two duplicates are fine", "a\na\nb\n", 2},
		{"blank lines break a run", "a\n\na\n\na\n", 1},
		{"run in the middle", "intro\n" + strings.Repeat("  - (or \"none\")\n", 13) + "outro\n", 13},
		{"varied lines of the same size", varyLines(13), 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, got := longestIdenticalRun(tt.prompt); got != tt.want {
				t.Errorf("longestIdenticalRun = %d, want %d", got, tt.want)
			}
		})
	}
}

func varyLines(n int) string {
	var b strings.Builder
	for i := range n {
		fmt.Fprintf(&b, "  - item %d\n", i)
	}
	return b.String()
}

// The measured trigger is a long run of byte-identical lines: the same prompt
// with that block replaced by varied lines of the same size answers normally.
// The hint has to name the repetition rather than send the caller to check
// source state, which is where the old message pointed.
func TestEmptyChatResponseHint(t *testing.T) {
	repeated := "audit the tree\n" + strings.Repeat("  - (or \"none\")\n", 14)
	hint := emptyChatResponseHint(repeated, "nb")
	if !strings.Contains(hint, "14 times in a row") {
		t.Errorf("hint %q does not report the run length", hint)
	}
	if !strings.Contains(hint, `(or \"none\")`) {
		t.Errorf("hint %q does not quote the repeated line", hint)
	}

	hint = emptyChatResponseHint("audit the tree\n"+varyLines(20), "nb")
	if !strings.Contains(hint, "transient") {
		t.Errorf("hint %q does not advise a retry", hint)
	}
	if !strings.Contains(hint, "nlm source list nb") {
		t.Errorf("hint %q does not name a runnable source check", hint)
	}

	// A very long repeated line is quoted in abbreviated form so the error
	// stays one readable line.
	long := strings.Repeat(strings.Repeat("x", 200)+"\n", 10)
	if hint := emptyChatResponseHint(long, "nb"); len(hint) > 240 {
		t.Errorf("hint is %d bytes, want an abbreviated quote: %q", len(hint), hint)
	}
}

// An empty answer is retryable, and a retry loop keyed on exit-class must not
// see the same class for an expired session.
func TestEmptyChatResponseExitClass(t *testing.T) {
	err := fmt.Errorf("generate chat: %w; %s", errEmptyChatResponse, emptyChatResponseHint("hi", "nb"))
	if got := exitCodeFor(err); got != exitTransient {
		t.Errorf("empty response exit = %d, want %d (transient)", got, exitTransient)
	}
}
