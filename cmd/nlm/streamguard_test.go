package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// repeat is a synthetic degenerate stream: the same block emitted over and
// over, which is the shape the runaway generate-chat run took.
func repeat(unit string, n int) string {
	return strings.Repeat(unit, n)
}

// longValidAnswer builds a large, legitimate answer: distinct table rows,
// prose, and a trailing citation list. It is well past the repetition guard's
// arming threshold and must never trip it.
func longValidAnswer(rows int) string {
	var b strings.Builder
	b.WriteString("| id | file | note |\n| --- | --- | --- |\n")
	for i := 0; i < rows; i++ {
		fmt.Fprintf(&b, "| %d | internal/pkg/file_%d.go | the resolver caches source %d across the run |\n", i, i, i)
	}
	for i := 0; i < rows; i++ {
		fmt.Fprintf(&b, "\nSection %d discusses how the loader handles case %d and why the fallback path stays bounded.\n", i, i)
	}
	for i := 0; i < rows; i++ {
		fmt.Fprintf(&b, "  [%d] answer %d-%d\n      p=0.9%d 46870d5a \"bundle %d\"\n", i, i*10, i*10+40, i%10, i)
	}
	return b.String()
}

func TestNewStreamGuardEnv(t *testing.T) {
	tests := []struct {
		name       string
		env        map[string]string
		wantBytes  int
		wantRepeat bool
	}{
		{name: "defaults", wantBytes: defaultMaxAnswerBytes, wantRepeat: true},
		{name: "custom size", env: map[string]string{"NLM_MAX_CHAT_BYTES": "4096"}, wantBytes: 4096, wantRepeat: true},
		{name: "size off", env: map[string]string{"NLM_MAX_CHAT_BYTES": "off"}, wantBytes: 0, wantRepeat: true},
		{name: "size zero", env: map[string]string{"NLM_MAX_CHAT_BYTES": "0"}, wantBytes: 0, wantRepeat: true},
		{name: "junk size keeps default", env: map[string]string{"NLM_MAX_CHAT_BYTES": "lots"}, wantBytes: defaultMaxAnswerBytes, wantRepeat: true},
		{name: "repeat off", env: map[string]string{"NLM_CHAT_REPEAT_GUARD": "off"}, wantBytes: defaultMaxAnswerBytes},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			g := newStreamGuard(func(k string) string { return test.env[k] })
			if g.maxBytes != test.wantBytes || g.repeatEnable != test.wantRepeat {
				t.Fatalf("guard = %+v, want maxBytes=%d repeat=%v", g, test.wantBytes, test.wantRepeat)
			}
		})
	}
}

func TestStreamGuardSizeLimit(t *testing.T) {
	g := streamGuard{maxBytes: 1024}
	if err := g.check(strings.Repeat("a", 1024)); err != nil {
		t.Fatalf("at limit = %v, want nil", err)
	}
	err := g.check(strings.Repeat("a", 1025))
	if !isRunawayOutput(err) {
		t.Fatalf("over limit = %v, want runaway error", err)
	}
	if !strings.Contains(err.Error(), "NLM_MAX_CHAT_BYTES") {
		t.Fatalf("error %q does not name the override", err)
	}
	// Delivered bytes are counted independently of the answer's length: a
	// stream that revises earlier text can ship far more than it accumulates.
	if err := g.checkDelivered(1024); err != nil {
		t.Fatalf("delivered at limit = %v, want nil", err)
	}
	if err := g.checkDelivered(4096); !isRunawayOutput(err) {
		t.Fatalf("delivered over limit = %v, want runaway error", err)
	}
	off := streamGuard{maxBytes: 0}
	if err := off.check(strings.Repeat("a", 1<<20)); err != nil {
		t.Fatalf("disabled size guard = %v, want nil", err)
	}
	if err := off.checkDelivered(1 << 30); err != nil {
		t.Fatalf("disabled delivered guard = %v, want nil", err)
	}
}

func TestStreamGuardRepetition(t *testing.T) {
	block := "| threshold | 0.05 | the MMD squared value stays below zero which is impossible |\n" +
		"and the same paragraph is emitted again with identical wording every time.\n"
	tests := []struct {
		name string
		text string
		want bool
	}{
		{
			name: "degenerate repeated block",
			text: longValidAnswer(20) + repeat(block, 200),
			want: true,
		},
		{
			name: "repetition under the arming threshold is ignored",
			text: repeat(block, 6),
		},
		{
			name: "long valid answer with tables, prose and citations",
			text: longValidAnswer(2000),
		},
		{
			name: "short repeated markdown rules do not trip",
			text: longValidAnswer(200) + repeat("| --- | --- |\n", 12),
		},
		{
			name: "repeated multibyte block trips without rune damage",
			text: longValidAnswer(20) + repeat("表 | 指標 | 0.05 | 同じ段落が何度も繰り返される 🌀🌀🌀🌀🌀🌀\n", 400),
			want: true,
		},
		{
			name: "long valid multibyte prose does not trip",
			text: func() string {
				var b strings.Builder
				for i := 0; i < 3000; i++ {
					fmt.Fprintf(&b, "第%d節では、ローダーがケース%dをどのように扱うかを説明します。🌀\n", i, i)
				}
				return b.String()
			}(),
		},
	}
	g := streamGuard{repeatEnable: true}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := g.check(test.text)
			if got := isRunawayOutput(err); got != test.want {
				t.Fatalf("check tripped = %v (%v), want %v", got, err, test.want)
			}
			if test.want && !strings.Contains(err.Error(), "NLM_CHAT_REPEAT_GUARD") {
				t.Fatalf("error %q does not name the override", err)
			}
			// The guard must be off when disabled, whatever the text.
			if err := (streamGuard{}).check(test.text); err != nil {
				t.Fatalf("disabled guard = %v, want nil", err)
			}
		})
	}
}

// The guard sees the cumulative answer, so a repeating run that starts and
// ends mid-chunk — chunk boundaries falling at arbitrary byte offsets, inside
// multibyte runes included — is detected exactly as if it arrived whole.
func TestStreamGuardRepetitionAcrossChunkBoundaries(t *testing.T) {
	full := longValidAnswer(20) + repeat("段落 repeated verbatim 🌀 with padding to exceed the minimum unit length ....\n", 400)
	g := streamGuard{repeatEnable: true}
	var acc strings.Builder
	tripped := false
	for i := 0; i < len(full); i += 37 { // 37 is coprime with the block length and splits runes
		end := i + 37
		if end > len(full) {
			end = len(full)
		}
		acc.WriteString(full[i:end])
		if isRunawayOutput(g.check(acc.String())) {
			tripped = true
			break
		}
	}
	if !tripped {
		t.Fatal("guard never tripped on a chunked degenerate stream")
	}
	if acc.Len() == len(full) {
		t.Fatal("guard tripped only at the very end; it should stop the stream early")
	}
}

// The transport reports its own error once the guard cancels the request
// context. The guard's verdict must still classify the run.
func TestRunawayErrorWinsOverCancellation(t *testing.T) {
	guardErr := &runawayOutputError{reason: "repetition", detail: "repeated block", bytes: 4096}
	err := fmt.Errorf("stream chat: %w (transport: %w)", guardErr, context.Canceled)
	if !isRunawayOutput(err) {
		t.Fatal("wrapped guard error not detected")
	}
	if got := exitCodeFor(err); got != exitRunaway {
		t.Fatalf("exit code = %d, want %d", got, exitRunaway)
	}
	if got := exitCodeName(exitRunaway); got != "runaway-output" {
		t.Fatalf("exit class = %q, want runaway-output", got)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation cause lost from the chain")
	}
}

func TestReportRunawayOutput(t *testing.T) {
	var buf bytes.Buffer
	reportRunawayOutput(&buf, &runawayOutputError{detail: "repeated block", bytes: 9000}, 4096)
	got := buf.String()
	for _, want := range []string{"runaway chat output", "INCOMPLETE", "not saved as an answer", "4096"} {
		if !strings.Contains(got, want) {
			t.Fatalf("report = %q, missing %q", got, want)
		}
	}
}
