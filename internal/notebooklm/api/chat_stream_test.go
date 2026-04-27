package api

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseChatResponseChunked(t *testing.T) {
	stream := mockChatStream(t,
		"**Thinking**",
		"Answer",
		"Answer continued",
	)

	var got []ChatChunk
	c := &Client{}
	err := c.parseChatResponseChunked(strings.NewReader(stream), nil, func(chunk ChatChunk) bool {
		got = append(got, chunk)
		return true
	})
	if err != nil {
		t.Fatalf("parseChatResponseChunked() error = %v", err)
	}

	want := []ChatChunk{
		{Phase: ChatChunkThinking, Header: "**Thinking**", Text: "**Thinking**"},
		{Phase: ChatChunkAnswer, Text: "Answer"},
		{Phase: ChatChunkAnswer, Text: " continued"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d chunks, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i].Phase != want[i].Phase || got[i].Header != want[i].Header || got[i].Text != want[i].Text {
			t.Fatalf("chunk %d = %#v, want %#v", i, got[i], want[i])
		}
	}
}

func TestParseChatResponseChunkedUsesWirePhaseForBoldAnswer(t *testing.T) {
	stream := mockChatStreamPayloads(t,
		mockChatPayload("**Thinking**\nWorking", chatWirePhaseThinking),
		mockChatPayload("**[Architect Persona]**\nYes", chatWirePhaseAnswer),
		mockChatPayload("**[Architect Persona]**\nYes.", chatWirePhaseAnswer),
	)

	var got []ChatChunk
	c := &Client{}
	err := c.parseChatResponseChunked(strings.NewReader(stream), nil, func(chunk ChatChunk) bool {
		got = append(got, chunk)
		return true
	})
	if err != nil {
		t.Fatalf("parseChatResponseChunked() error = %v", err)
	}

	want := []ChatChunk{
		{Phase: ChatChunkThinking, Header: "**Thinking**", Text: "**Thinking**\nWorking"},
		{Phase: ChatChunkAnswer, Text: "**[Architect Persona]**\nYes"},
		{Phase: ChatChunkAnswer, Text: "."},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d chunks, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i].Phase != want[i].Phase || got[i].Header != want[i].Header || got[i].Text != want[i].Text {
			t.Fatalf("chunk %d = %#v, want %#v", i, got[i], want[i])
		}
	}
}

func TestAnswerOnlyCallback(t *testing.T) {
	var got []string
	callback := answerOnlyCallback(func(chunk string) bool {
		got = append(got, chunk)
		return true
	})

	for _, chunk := range []ChatChunk{
		{Phase: ChatChunkThinking, Text: "**Thinking**"},
		{Phase: ChatChunkAnswer, Text: "Answer"},
		{Phase: ChatChunkAnswer, Text: " continued"},
		{Phase: ChatChunkAnswer, Text: ""},
	} {
		if !callback(chunk) {
			t.Fatalf("callback returned false for %#v", chunk)
		}
	}

	want := []string{"Answer", " continued"}
	if len(got) != len(want) {
		t.Fatalf("got %d chunks, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("chunk %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestBuildChatArgsUsesProtoBackedConversationState(t *testing.T) {
	t.Parallel()

	c := &Client{}
	argsJSON, err := c.buildChatArgs(ChatRequest{
		ProjectID:      "project-123",
		Prompt:         "What changed?",
		SourceIDs:      []string{"src-1", "src-2"},
		ConversationID: "conv-123",
		History: []ChatMessage{
			{Content: "Earlier question", Role: 1},
			{Content: "Earlier answer", Role: 2},
		},
		SeqNum: 7,
	})
	if err != nil {
		t.Fatalf("buildChatArgs() error = %v", err)
	}

	var got []interface{}
	if err := json.Unmarshal([]byte(argsJSON), &got); err != nil {
		t.Fatalf("unmarshal args: %v", err)
	}

	if len(got) != 9 {
		t.Fatalf("len(args) = %d, want 9", len(got))
	}
	if got[1] != "What changed?" {
		t.Fatalf("prompt = %v, want %q", got[1], "What changed?")
	}
	if got[4] != "conv-123" {
		t.Fatalf("conversation_id = %v, want %q", got[4], "conv-123")
	}
	if got[7] != "project-123" {
		t.Fatalf("notebook_id = %v, want %q", got[7], "project-123")
	}
	if got[8] != float64(7) {
		t.Fatalf("sequence_number = %v, want 7", got[8])
	}

	history, ok := got[2].([]interface{})
	if !ok || len(history) != 2 {
		t.Fatalf("history = %#v, want 2 entries", got[2])
	}
	first, ok := history[0].([]interface{})
	if !ok || len(first) != 3 {
		t.Fatalf("history[0] = %#v", history[0])
	}
	if first[0] != "Earlier question" || first[2] != float64(1) {
		t.Fatalf("history[0] = %#v, want content/role preserved", first)
	}
}

func mockChatStream(t *testing.T, texts ...string) string {
	t.Helper()

	payloads := make([]interface{}, 0, len(texts))
	for _, text := range texts {
		payloads = append(payloads, []interface{}{[]interface{}{text}})
	}
	return mockChatStreamPayloads(t, payloads...)
}

func mockChatStreamPayloads(t *testing.T, payloads ...interface{}) string {
	t.Helper()

	var b strings.Builder
	b.WriteString(")]}'\n")
	for _, payload := range payloads {
		inner, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal inner chunk: %v", err)
		}
		envelope, err := json.Marshal([]interface{}{"wrb.fr", "mock", string(inner)})
		if err != nil {
			t.Fatalf("marshal envelope: %v", err)
		}
		b.WriteString("1\n")
		b.Write(envelope)
		b.WriteByte('\n')
	}
	return b.String()
}

func mockChatPayload(text string, phase int) interface{} {
	return []interface{}{
		[]interface{}{
			text,
			nil,
			[]interface{}{"conv", "resp", float64(1)},
			nil,
			[]interface{}{},
			nil,
			nil,
			nil,
			phase,
		},
	}
}

// TestParseCitationsV2SlotOrdering locks in the invariant that Citation.SourceIndex
// is the 1-based *slot* number (matching [N] in the narrative), not the project
// index of the cited source. Regression: a run with a 100+-source notebook had
// narrative [1] referring to the first thing the model cited (e.g. slot-1 was
// src at project-index 99), while the footer printed "[1] = project-index-0"
// because SourceIndex was srcIdx+1. See /tmp/nlm-impl-count.log for the repro.
func TestParseCitationsV2SlotOrdering(t *testing.T) {
	// Three sources in the project list, and three emitted citation slots
	// that reference project indices in a non-monotonic order:
	//   slot 0 (narrative [1]) → project-index 2 (src_c)
	//   slot 1 (narrative [2]) → project-index 0 (src_a)
	//   slot 2 (narrative [3]) → project-indices 1,2 (src_b AND src_c together)
	sourceIDs := []string{"src_a", "src_b", "src_c"}
	mappingData := []interface{}{
		[]interface{}{[]interface{}{nil, float64(0), float64(10)}, []interface{}{float64(2)}},
		[]interface{}{[]interface{}{nil, float64(11), float64(20)}, []interface{}{float64(0)}},
		[]interface{}{[]interface{}{nil, float64(21), float64(30)}, []interface{}{float64(1), float64(2)}},
	}
	citationData := []interface{}{
		[]interface{}{nil, nil, float64(0.9), nil, nil},
		[]interface{}{nil, nil, float64(0.8), nil, nil},
		[]interface{}{nil, nil, float64(0.7), nil, nil},
	}

	got := parseCitationsV2(citationData, mappingData, sourceIDs)
	// One citation per (slot, srcIdx) pair: slots 0+1 have one src each,
	// slot 2 has two → 4 total.
	if len(got) != 4 {
		t.Fatalf("got %d citations, want 4: %+v", len(got), got)
	}
	want := []Citation{
		{SourceIndex: 1, SourceID: "src_c", StartChar: 0, EndChar: 10, Confidence: 0.9},
		{SourceIndex: 2, SourceID: "src_a", StartChar: 11, EndChar: 20, Confidence: 0.8},
		{SourceIndex: 3, SourceID: "src_b", StartChar: 21, EndChar: 30, Confidence: 0.7},
		{SourceIndex: 3, SourceID: "src_c", StartChar: 21, EndChar: 30, Confidence: 0.7},
	}
	for i, w := range want {
		g := got[i]
		if g.SourceIndex != w.SourceIndex || g.SourceID != w.SourceID ||
			g.StartChar != w.StartChar || g.EndChar != w.EndChar ||
			g.Confidence != w.Confidence {
			t.Errorf("citation %d = %+v, want %+v", i, g, w)
		}
	}
}

// TestParseCitationsV2SkipsUnresolvableSrcIdx exercises the case where
// the server emits a srcIdx past the end of the request's source list —
// observed when --source-ids narrows to a subset and the server still
// references the original full set. A Citation we can't resolve to a
// SourceID is unusable downstream, so the parser drops it rather than
// emitting a blank footer line.
func TestParseCitationsV2SkipsUnresolvableSrcIdx(t *testing.T) {
	sourceIDs := []string{"src_a"} // request narrowed to one source
	mappingData := []interface{}{
		// Slot 0: srcIdx 0 (resolves to src_a).
		[]interface{}{[]interface{}{nil, float64(0), float64(10)}, []interface{}{float64(0)}},
		// Slot 1: srcIdx 5 (out of range — must be dropped).
		[]interface{}{[]interface{}{nil, float64(11), float64(20)}, []interface{}{float64(5)}},
		// Slot 2: mixes valid (0) and invalid (3) — the valid one survives.
		[]interface{}{[]interface{}{nil, float64(21), float64(30)}, []interface{}{float64(0), float64(3)}},
	}
	citationData := []interface{}{
		[]interface{}{nil, nil, float64(0.9), nil, nil},
		[]interface{}{nil, nil, float64(0.8), nil, nil},
		[]interface{}{nil, nil, float64(0.7), nil, nil},
	}

	got := parseCitationsV2(citationData, mappingData, sourceIDs)
	want := []Citation{
		{SourceIndex: 1, SourceID: "src_a", StartChar: 0, EndChar: 10, Confidence: 0.9},
		{SourceIndex: 3, SourceID: "src_a", StartChar: 21, EndChar: 30, Confidence: 0.7},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d citations, want %d: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		g := got[i]
		if g.SourceIndex != w.SourceIndex || g.SourceID != w.SourceID ||
			g.StartChar != w.StartChar || g.EndChar != w.EndChar ||
			g.Confidence != w.Confidence {
			t.Errorf("citation %d = %+v, want %+v", i, g, w)
		}
	}
	for _, c := range got {
		if c.SourceID == "" {
			t.Errorf("citation with empty SourceID leaked through: %+v", c)
		}
	}
}
