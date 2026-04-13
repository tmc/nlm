package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/tmc/nlm/internal/notebooklm/api"
)

func TestChatStreamRendererNonTTYDropsThinkingOutput(t *testing.T) {
	var out bytes.Buffer
	var status bytes.Buffer

	r := newChatStreamRenderer(&out, &status, false, false)
	r.WriteChunk(api.ChatChunk{
		Phase:  api.ChatChunkThinking,
		Header: "**Thinking**",
		Text:   "**Thinking**\nPlanning response",
	})
	r.WriteChunk(api.ChatChunk{
		Phase: api.ChatChunkAnswer,
		Text:  "Hello, world.",
	})
	r.Finish()

	if got := out.String(); got != "Hello, world." {
		t.Fatalf("answer output = %q, want %q", got, "Hello, world.")
	}
	if got := status.String(); got != "" {
		t.Fatalf("status output = %q, want empty", got)
	}
	if got := r.Thinking(); got != "**Thinking**\nPlanning response\n" {
		t.Fatalf("thinking trace = %q", got)
	}
}

func TestChatStreamRendererThinkingModes(t *testing.T) {
	t.Run("header-only", func(t *testing.T) {
		var out bytes.Buffer
		var status bytes.Buffer

		r := newChatStreamRenderer(&out, &status, true, false)
		r.WriteChunk(api.ChatChunk{
			Phase:  api.ChatChunkThinking,
			Header: "**Thinking**",
			Text:   "**Thinking**\nPlanning response",
		})
		r.WriteChunk(api.ChatChunk{
			Phase: api.ChatChunkAnswer,
			Text:  "Answer",
		})
		r.Finish()

		if got := out.String(); got != "Answer" {
			t.Fatalf("answer output = %q, want %q", got, "Answer")
		}
		if got := status.String(); !strings.Contains(got, "[thinking] Thinking") {
			t.Fatalf("status output = %q, want thinking header", got)
		}
		if got := status.String(); !strings.Contains(got, "\r") {
			t.Fatalf("status output = %q, want carriage-return clear", got)
		}
	})

	t.Run("non-tty-with-thinking-flag", func(t *testing.T) {
		var out bytes.Buffer
		var status bytes.Buffer

		r := newChatStreamRenderer(&out, &status, true, false)
		r.WriteChunk(api.ChatChunk{
			Phase:  api.ChatChunkThinking,
			Header: "**Thinking**",
			Text:   "**Thinking**\nPlanning response",
		})
		r.WriteChunk(api.ChatChunk{
			Phase: api.ChatChunkAnswer,
			Text:  "Answer",
		})
		r.Finish()

		if got := out.String(); got != "Answer" {
			t.Fatalf("answer output = %q, want %q", got, "Answer")
		}
		if got := status.String(); !strings.Contains(got, "[thinking] Thinking") {
			t.Fatalf("status output = %q, want thinking header", got)
		}
	})

	t.Run("verbose", func(t *testing.T) {
		var out bytes.Buffer
		var status bytes.Buffer

		r := newChatStreamRenderer(&out, &status, true, true)
		r.WriteChunk(api.ChatChunk{
			Phase:  api.ChatChunkThinking,
			Header: "**Thinking**",
			Text:   "**Thinking**\nPlanning response",
		})

		got := status.String()
		if !strings.Contains(got, ansiGrey+"**Thinking**\nPlanning response"+ansiReset+"\n") {
			t.Fatalf("status output = %q, want verbose thinking trace", got)
		}
	})
}
