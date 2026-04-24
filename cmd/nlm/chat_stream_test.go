package main

import (
	"bytes"
	"encoding/json"
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
	// Footer consistently uses [N] brackets for all entries — the inline
	// superscript in the answer already marks which ones got spliced, and a
	// single superscript among brackets reads as a bug.
	if !strings.Contains(s, "[1] src_aged") {
		t.Fatalf("footer missing aged-out entry with bracket marker: %q", s)
	}
	if !strings.Contains(s, "[2] src_tail") {
		t.Fatalf("footer missing held entry with bracket marker: %q", s)
	}
	if strings.Contains(s, "² src_tail") {
		t.Fatalf("footer should not mix superscript markers: %q", s)
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

func TestSnapToWordBoundary(t *testing.T) {
	cases := []struct {
		name string
		text string
		pos  int
		want int
	}{
		{"already at space", "hello world", 5, 5},
		{"mid-word advances to space", "answers are", 4, 7},      // "answ|ers" → "answers|"
		{"mid-word to punctuation", "parser.go uses", 4, 6},      // "pars|er.go" → "parser|.go"
		{"backtick is word-ish, snaps past", "foo`bar baz", 3, 7}, // "foo|`bar baz" treated as word, scans to space
		{"end of string", "hello", 5, 5},
		{"past end clamps to len", "hello", 10, 5},
		{"negative clamps to 0", "hello", -1, 0},
		{"no boundary within 32 returns original", strings.Repeat("x", 40), 3, 3},
	}
	for _, tc := range cases {
		if got := snapToWordBoundary(tc.text, tc.pos); got != tc.want {
			t.Errorf("%s: snapToWordBoundary(%q, %d) = %d, want %d", tc.name, tc.text, tc.pos, got, tc.want)
		}
	}
}

func TestInsertSuperscriptsClustersToBrackets(t *testing.T) {
	// Two citations sharing the same splice position must render as a
	// bracketed cluster to avoid digit ambiguity ("³⁴" reading as "3⁴").
	answer := "Hello world."
	citations := []api.Citation{
		{SourceIndex: 3, SourceID: "a", EndChar: 5},
		{SourceIndex: 4, SourceID: "b", EndChar: 5},
	}
	got := insertSuperscripts(answer, citations)
	want := "Hello[3,4] world."
	if got != want {
		t.Errorf("cluster = %q, want %q", got, want)
	}
}

func TestInsertSuperscriptsSingletonStaysSuperscript(t *testing.T) {
	// A single citation stays a Unicode superscript.
	answer := "Hello world."
	citations := []api.Citation{{SourceIndex: 3, SourceID: "a", EndChar: 5}}
	got := insertSuperscripts(answer, citations)
	want := "Hello³ world."
	if got != want {
		t.Errorf("singleton = %q, want %q", got, want)
	}
}

func TestInsertSuperscriptsSnapsToWordBoundary(t *testing.T) {
	// EndChar 4 lands inside "answers"; splice must snap to the end of the word.
	answer := "Final answers are cited."
	citations := []api.Citation{{SourceIndex: 1, SourceID: "s", EndChar: 10}} // mid-word "answ|ers"
	got := insertSuperscripts(answer, citations)
	want := "Final answers¹ are cited."
	if got != want {
		t.Errorf("insertSuperscripts snap = %q, want %q", got, want)
	}
}

func TestRenderPersistedAssistantBlock(t *testing.T) {
	var out, status bytes.Buffer
	msg := ChatMessage{
		Role:    "assistant",
		Content: "Answer body.",
		Citations: []api.Citation{
			{SourceIndex: 1, SourceID: "src_a", Title: "First"},
			{SourceIndex: 2, SourceID: "src_b", Title: "Second"},
		},
	}
	renderPersistedAssistant(&out, &status, msg, citationModeBlock)
	if !strings.Contains(out.String(), "Answer body.") {
		t.Fatalf("body missing from stdout: %q", out.String())
	}
	s := status.String()
	if !strings.Contains(s, "Sources:") {
		t.Fatalf("block footer missing: %q", s)
	}
	if !strings.Contains(s, "[1] src_a") || !strings.Contains(s, "First") {
		t.Fatalf("entry 1 missing: %q", s)
	}
	if !strings.Contains(s, "[2] src_b") || !strings.Contains(s, "Second") {
		t.Fatalf("entry 2 missing: %q", s)
	}
}

func TestRenderPersistedAssistantOverlay(t *testing.T) {
	var out, status bytes.Buffer
	msg := ChatMessage{
		Role:    "assistant",
		Content: "Hello world.",
		Citations: []api.Citation{
			{SourceIndex: 1, SourceID: "src_a", Title: "greeting", EndChar: 5},
		},
	}
	renderPersistedAssistant(&out, &status, msg, citationModeOverlay)
	if !strings.Contains(out.String(), "Hello¹") {
		t.Fatalf("overlay superscript missing (post-snap): %q", out.String())
	}
	if !strings.Contains(status.String(), "¹ src_a") {
		t.Fatalf("overlay footer missing entry: %q", status.String())
	}
}

func TestRenderPersistedAssistantNoCitations(t *testing.T) {
	var out, status bytes.Buffer
	msg := ChatMessage{Role: "assistant", Content: "Plain answer."}
	renderPersistedAssistant(&out, &status, msg, citationModeBlock)
	if got := out.String(); !strings.HasPrefix(got, "Plain answer.") {
		t.Fatalf("body missing: %q", got)
	}
	if status.String() != "" {
		t.Fatalf("footer should be empty for no citations, got %q", status.String())
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

// parseJSONLEvents splits newline-delimited JSON on stdout into a slice of
// decoded events. Any non-object line fails the calling test.
func parseJSONLEvents(t *testing.T, raw string) []map[string]any {
	t.Helper()
	lines := strings.Split(strings.TrimRight(raw, "\n"), "\n")
	events := make([]map[string]any, 0, len(lines))
	for i, line := range lines {
		if line == "" {
			continue
		}
		var ev map[string]any
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatalf("line %d not valid JSON: %v\nline=%q", i, err, line)
		}
		events = append(events, ev)
	}
	return events
}

func TestChatStreamRendererJSONLEmitsTypedEvents(t *testing.T) {
	var out, status bytes.Buffer
	r := newChatStreamRenderer(&out, &status, false, false, citationModeOff)
	r.jsonl = true
	r.jsonlIncludeThinking = true

	r.WriteChunk(api.ChatChunk{
		Phase:  api.ChatChunkThinking,
		Header: "**Thinking**",
		Text:   "**Thinking**\nPlanning response",
	})
	r.WriteChunk(api.ChatChunk{
		Phase: api.ChatChunkAnswer,
		Text:  "Hello, ",
	})
	r.WriteChunk(api.ChatChunk{
		Phase: api.ChatChunkAnswer,
		Text:  "world.",
		Citations: []api.Citation{
			{SourceIndex: 1, SourceID: "src_aaa", Title: "Guide", StartChar: 0, EndChar: 12, Confidence: 0.87},
		},
		FollowUps: []string{"Tell me more", "Different angle"},
	})
	r.Finish()

	// Human status stream must stay empty under jsonl mode — no ANSI chatter.
	if got := status.String(); got != "" {
		t.Fatalf("jsonl mode leaked status output: %q", got)
	}

	events := parseJSONLEvents(t, out.String())
	if len(events) < 5 {
		t.Fatalf("expected at least 5 events (thinking, 2×answer, citation, done), got %d: %+v", len(events), events)
	}

	// First event is the thinking trace.
	if events[0]["phase"] != "thinking" {
		t.Fatalf("events[0].phase = %v, want thinking", events[0]["phase"])
	}
	if !strings.Contains(events[0]["text"].(string), "Planning response") {
		t.Fatalf("events[0].text missing trace body: %v", events[0]["text"])
	}

	// Second + third are answer deltas preserving order.
	if events[1]["phase"] != "answer" || events[1]["text"] != "Hello, " {
		t.Fatalf("events[1] = %v, want answer 'Hello, '", events[1])
	}
	if events[2]["phase"] != "answer" || events[2]["text"] != "world." {
		t.Fatalf("events[2] = %v, want answer 'world.'", events[2])
	}

	// Fourth is the citation event.
	if events[3]["phase"] != "citation" {
		t.Fatalf("events[3].phase = %v, want citation", events[3]["phase"])
	}
	if events[3]["source_id"] != "src_aaa" {
		t.Fatalf("citation source_id = %v, want src_aaa", events[3]["source_id"])
	}
	if got, want := events[3]["confidence"].(float64), 0.87; got != want {
		t.Fatalf("citation confidence = %v, want %v", got, want)
	}

	// Last event is done, preceded by followup events.
	last := events[len(events)-1]
	if last["phase"] != "done" {
		t.Fatalf("last event = %v, want phase=done", last)
	}
	sawFollowup := 0
	for _, ev := range events {
		if ev["phase"] == "followup" {
			sawFollowup++
		}
	}
	if sawFollowup != 2 {
		t.Fatalf("expected 2 followup events, got %d", sawFollowup)
	}
}

func TestChatStreamRendererJSONLCumulativeThinkingNotDuplicated(t *testing.T) {
	var out, status bytes.Buffer
	r := newChatStreamRenderer(&out, &status, false, false, citationModeOff)
	r.jsonl = true
	r.jsonlIncludeThinking = true

	// Thinking arrives as cumulative snapshots; jsonl must emit once per
	// change, not once per snapshot.
	r.WriteChunk(api.ChatChunk{Phase: api.ChatChunkThinking, Text: "step 1"})
	r.WriteChunk(api.ChatChunk{Phase: api.ChatChunkThinking, Text: "step 1"}) // dup
	r.WriteChunk(api.ChatChunk{Phase: api.ChatChunkThinking, Text: "step 1\nstep 2"})
	r.Finish()

	events := parseJSONLEvents(t, out.String())
	thinkingCount := 0
	for _, ev := range events {
		if ev["phase"] == "thinking" {
			thinkingCount++
		}
	}
	if thinkingCount != 2 {
		t.Fatalf("thinking events = %d, want 2 (duplicate snapshot should not re-emit)", thinkingCount)
	}
}

func TestChatStreamRendererJSONLIsOptIn(t *testing.T) {
	// Without r.jsonl, output must match existing human-readable behavior
	// byte-for-byte — no regressions for users who didn't ask for JSON.
	var out, status bytes.Buffer
	r := newChatStreamRenderer(&out, &status, false, false, citationModeOff)
	r.WriteChunk(api.ChatChunk{Phase: api.ChatChunkAnswer, Text: "plain answer"})
	r.Finish()

	if got := out.String(); got != "plain answer" {
		t.Fatalf("non-jsonl stdout = %q, want plain answer (jsonl mode leaked into default path)", got)
	}
}

func TestChatStreamRendererJSONLGatesThinking(t *testing.T) {
	// --citations=json without --thinking suppresses thinking events on stdout
	// while still emitting answer + citation JSON-lines events.
	var out, status bytes.Buffer
	r := newChatStreamRenderer(&out, &status, false, false, citationModeJSON)
	r.jsonl = true
	// jsonlIncludeThinking intentionally left false.

	r.WriteChunk(api.ChatChunk{Phase: api.ChatChunkThinking, Text: "hidden trace"})
	r.WriteChunk(api.ChatChunk{
		Phase: api.ChatChunkAnswer,
		Text:  "answer body",
		Citations: []api.Citation{
			{SourceIndex: 1, SourceID: "s1", Title: "t", StartChar: 0, EndChar: 6},
		},
	})
	r.Finish()

	events := parseJSONLEvents(t, out.String())
	for _, ev := range events {
		if ev["phase"] == "thinking" {
			t.Fatalf("thinking event leaked into jsonl output without --thinking: %v", ev)
		}
	}
	sawAnswer, sawCitation := false, false
	for _, ev := range events {
		switch ev["phase"] {
		case "answer":
			sawAnswer = true
		case "citation":
			sawCitation = true
		}
	}
	if !sawAnswer || !sawCitation {
		t.Fatalf("expected answer + citation events, got %+v", events)
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
			name:     "nothing streamed prints full response",
			streamed: "",
			full:     "Hello.\nWorld.\n",
			want:     "Hello.\nWorld.\n",
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
			want:     "",
		},
		{
			name:     "divergent fallback emits boundary and full response",
			streamed: "Section 1: intro.\nSection 2: ",
			full:     "Totally different answer.\n",
			want:     "\n--- streaming failed, re-rendering full response ---\nTotally different answer.\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			printStreamFallback(&buf, tt.streamed, tt.full, tt.jsonl)
			if got := buf.String(); got != tt.want {
				t.Fatalf("printStreamFallback out = %q, want %q", got, tt.want)
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
	var ev map[string]any
	if err := json.Unmarshal([]byte(line), &ev); err != nil {
		t.Fatalf("fallback event is not valid JSON: %v (%q)", err, line)
	}
	if ev["phase"] != "fallback" {
		t.Fatalf("phase = %v, want fallback", ev["phase"])
	}
	if ev["text"] != "full fallback text" {
		t.Fatalf("text = %v, want full fallback text", ev["text"])
	}
}
