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

	r := newChatStreamRenderer(&out, &status, false, false, citationModeOff)
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
	if got := r.Thinking(); got != "**Thinking**\nPlanning response" {
		t.Fatalf("thinking trace = %q", got)
	}
}

func TestChatStreamRendererThinkingReplacesCumulativeSnapshots(t *testing.T) {
	var out, status bytes.Buffer
	r := newChatStreamRenderer(&out, &status, false, false, citationModeOff)
	r.WriteChunk(api.ChatChunk{Phase: api.ChatChunkThinking, Header: "**T**", Text: "**T**\nShip"})
	r.WriteChunk(api.ChatChunk{Phase: api.ChatChunkThinking, Header: "**T**", Text: "**T**\nShip a thin wrapper"})
	r.WriteChunk(api.ChatChunk{Phase: api.ChatChunkThinking, Header: "**T**", Text: "**T**\nShip a thin wrapper via cmd/cove-serve"})
	r.Finish()

	want := "**T**\nShip a thin wrapper via cmd/cove-serve"
	if got := r.Thinking(); got != want {
		t.Fatalf("thinking trace = %q, want %q", got, want)
	}
}

func TestChatStreamRendererThinkingModes(t *testing.T) {
	t.Run("header-only", func(t *testing.T) {
		var out bytes.Buffer
		var status bytes.Buffer

		r := newChatStreamRenderer(&out, &status, true, false, citationModeOff)
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

		r := newChatStreamRenderer(&out, &status, true, false, citationModeOff)
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

		r := newChatStreamRenderer(&out, &status, true, true, citationModeOff)
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

func TestChatStreamRendererCitationModeBlock(t *testing.T) {
	var out, status bytes.Buffer
	r := newChatStreamRenderer(&out, &status, false, false, citationModeBlock)
	r.resolveTitle = func(id string) string {
		if id == "src_aaa" {
			return "Installation Guide"
		}
		return ""
	}
	r.WriteChunk(api.ChatChunk{
		Phase: api.ChatChunkAnswer,
		Text:  "Answer body.",
		Citations: []api.Citation{
			{SourceIndex: 1, SourceID: "src_aaa", Title: "ignored when resolver hits"},
			{SourceIndex: 2, SourceID: "src_bbb_longidentifier", Title: "Fallback excerpt"},
		},
	})
	r.Finish()

	if got := out.String(); got != "Answer body." {
		t.Fatalf("answer output = %q, want %q", got, "Answer body.")
	}
	s := status.String()
	if !strings.Contains(s, "Sources:") {
		t.Fatalf("status missing Sources header: %q", s)
	}
	if !strings.Contains(s, "[1] src_aaa") || !strings.Contains(s, "Installation Guide") {
		t.Fatalf("status missing resolved title: %q", s)
	}
	// Source IDs render in full (no truncation).
	if !strings.Contains(s, "src_bbb_longidentifier") {
		t.Fatalf("status missing full source id: %q", s)
	}
	// Falls back to server-supplied excerpt when resolver returns "".
	if !strings.Contains(s, "Fallback excerpt") {
		t.Fatalf("status missing fallback excerpt: %q", s)
	}
}

func TestChatStreamRendererCitationModeOverlay(t *testing.T) {
	var out, status bytes.Buffer
	r := newChatStreamRenderer(&out, &status, false, false, citationModeOverlay)
	// Stream the answer in two deltas; overlay mode must buffer until Finish.
	r.WriteChunk(api.ChatChunk{
		Phase: api.ChatChunkAnswer,
		Text:  "The service requires Go 1.22",
	})
	r.WriteChunk(api.ChatChunk{
		Phase: api.ChatChunkAnswer,
		Text:  " and uses TLS by default.",
		Citations: []api.Citation{
			{SourceIndex: 1, SourceID: "src_a", Title: "Go 1.22 required", StartChar: 12, EndChar: 28},
			{SourceIndex: 2, SourceID: "src_b", Title: "TLS enabled", StartChar: 29, EndChar: 52},
		},
	})

	// Nothing should have hit stdout yet under overlay mode.
	if got := out.String(); got != "" {
		t.Fatalf("overlay leaked during streaming: %q", got)
	}

	r.Finish()

	body := out.String()
	// The full answer must be present.
	if !strings.Contains(body, "The service requires Go 1.22") {
		t.Fatalf("answer missing: %q", body)
	}
	// Superscripts must appear at the end of each cited span.
	if !strings.Contains(body, "Go 1.22¹") {
		t.Fatalf("missing superscript 1 marker: %q", body)
	}
	if !strings.Contains(body, "default².") {
		t.Fatalf("missing superscript 2 marker: %q", body)
	}

	s := status.String()
	if !strings.Contains(s, "¹ src_a") || !strings.Contains(s, "Go 1.22 required") {
		t.Fatalf("footnote missing entry 1: %q", s)
	}
	if !strings.Contains(s, "² src_b") || !strings.Contains(s, "TLS enabled") {
		t.Fatalf("footnote missing entry 2: %q", s)
	}
}

func TestChatStreamRendererCitationModeOff(t *testing.T) {
	var out, status bytes.Buffer
	r := newChatStreamRenderer(&out, &status, false, false, citationModeOff)
	r.WriteChunk(api.ChatChunk{
		Phase: api.ChatChunkAnswer,
		Text:  "Answer.",
		Citations: []api.Citation{
			{SourceIndex: 1, SourceID: "src_a", Title: "noisy"},
		},
	})
	r.Finish()

	if got := status.String(); strings.Contains(got, "Sources:") {
		t.Fatalf("citation block leaked under off mode: %q", got)
	}
	if got := out.String(); got != "Answer." {
		t.Fatalf("answer = %q, want %q", got, "Answer.")
	}
}

func TestResolveCitationMode(t *testing.T) {
	cases := []struct {
		flag    string
		outTTY  bool
		want    citationRenderMode
		wantStr string
	}{
		{"", true, citationModeStream, "tty default = stream"},
		{"", false, citationModeOff, "pipe default = off"},
		{"block", false, citationModeBlock, "explicit block overrides pipe"},
		{"stream", true, citationModeStream, "stream"},
		{"inline-footer", true, citationModeStream, "inline-footer alias"},
		{"tail", true, citationModeTail, "tail"},
		{"overlay", true, citationModeOverlay, "overlay"},
		{"footnote", true, citationModeOverlay, "footnote alias"},
		{"off", true, citationModeOff, "explicit off"},
		{"none", true, citationModeOff, "none alias"},
		{"nonsense", true, citationModeStream, "unknown falls through to tty default"},
	}
	for _, tc := range cases {
		if got := resolveCitationMode(tc.flag, tc.outTTY); got != tc.want {
			t.Errorf("%s: resolveCitationMode(%q, %v) = %v, want %v", tc.wantStr, tc.flag, tc.outTTY, got, tc.want)
		}
	}
}

func TestChatStreamRendererCitationModeStream(t *testing.T) {
	var out, status bytes.Buffer
	r := newChatStreamRenderer(&out, &status, false, false, citationModeStream)
	r.WriteChunk(api.ChatChunk{Phase: api.ChatChunkAnswer, Text: "Hello "})
	r.WriteChunk(api.ChatChunk{Phase: api.ChatChunkAnswer, Text: "world."})
	// Stream must have flushed both chunks live.
	if got := out.String(); got != "Hello world." {
		t.Fatalf("stream live flush = %q, want %q", got, "Hello world.")
	}
	// Citations arrive late, only on Finish should they be emitted.
	r.citations = []api.Citation{
		{SourceIndex: 1, SourceID: "src_a", Title: "greeting", StartChar: 0, EndChar: 5},
		{SourceIndex: 2, SourceID: "src_b", Title: "target", StartChar: 6, EndChar: 12},
	}
	r.Finish()
	s := status.String()
	if !strings.Contains(s, "Citations:") {
		t.Fatalf("missing Citations footer: %q", s)
	}
	if !strings.Contains(s, "[1] chars 0-5") || !strings.Contains(s, "greeting") {
		t.Fatalf("missing entry 1 with range: %q", s)
	}
	if !strings.Contains(s, "[2] chars 6-12") || !strings.Contains(s, "target") {
		t.Fatalf("missing entry 2 with range: %q", s)
	}
	// Stream mode must not splice superscripts into the answer text.
	if strings.ContainsAny(out.String(), "¹²³") {
		t.Fatalf("stream mode leaked superscripts into answer: %q", out.String())
	}
}

func TestChatStreamRendererCitationModeTail(t *testing.T) {
	var out, status bytes.Buffer
	r := newChatStreamRenderer(&out, &status, false, false, citationModeTail)
	r.tailWindow = 16 // small window so we can test aging-out easily

	// First delta: 10 bytes. Window is 16, so nothing should flush yet.
	r.WriteChunk(api.ChatChunk{Phase: api.ChatChunkAnswer, Text: "0123456789"})
	if got := out.String(); got != "" {
		t.Fatalf("tail flushed too early: %q", got)
	}

	// Second delta: 20 more bytes (total 30). stable = 30-16 = 14 bytes should flush.
	r.WriteChunk(api.ChatChunk{Phase: api.ChatChunkAnswer, Text: "ABCDEFGHIJKLMNOPQRST"})
	if got := out.String(); got != "0123456789ABCD" {
		t.Fatalf("tail flush boundary wrong: got %q, want %q", got, "0123456789ABCD")
	}

	// Citation that lands in aged-out territory (EndChar=5) should spill to footer.
	// Citation that lands in the held tail (EndChar=30) should get inline splicing.
	r.citations = []api.Citation{
		{SourceIndex: 1, SourceID: "src_aged", Title: "aged out", StartChar: 0, EndChar: 5},
		{SourceIndex: 2, SourceID: "src_tail", Title: "still held", StartChar: 24, EndChar: 30},
	}
	r.Finish()

	body := out.String()
	if !strings.HasPrefix(body, "0123456789ABCD") {
		t.Fatalf("answer prefix changed unexpectedly: %q", body)
	}
	// Inline superscript should be at the end of the held tail (after 'T').
	if !strings.Contains(body, "EFGHIJKLMNOPQRST²") {
		t.Fatalf("inline superscript missing in held tail: %q", body)
	}
	// Aged-out citation should NOT appear inline.
	if strings.Contains(body, "¹") {
		t.Fatalf("aged-out citation leaked inline: %q", body)
	}

	s := status.String()
	// Footer marker: aged-out = bracket, still-held = superscript.
	if !strings.Contains(s, "[1] src_aged") {
		t.Fatalf("footer missing aged-out entry with bracket marker: %q", s)
	}
	if !strings.Contains(s, "² src_tail") {
		t.Fatalf("footer missing held entry with superscript marker: %q", s)
	}
}

func TestChatStreamRendererCitationModeTailNoCitations(t *testing.T) {
	// Tail mode with no citations should still flush the full answer on Finish.
	var out, status bytes.Buffer
	r := newChatStreamRenderer(&out, &status, false, false, citationModeTail)
	r.tailWindow = 8
	r.WriteChunk(api.ChatChunk{Phase: api.ChatChunkAnswer, Text: "short"})
	r.Finish()
	if got := out.String(); got != "short" {
		t.Fatalf("tail final flush = %q, want %q", got, "short")
	}
	if got := status.String(); got != "" {
		t.Fatalf("no-citation footer leaked: %q", got)
	}
}

func TestSuperscript(t *testing.T) {
	cases := map[int]string{
		0:  "",
		1:  "¹",
		2:  "²",
		10: "¹⁰",
		42: "⁴²",
	}
	for n, want := range cases {
		if got := superscript(n); got != want {
			t.Errorf("superscript(%d) = %q, want %q", n, got, want)
		}
	}
}
