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

	var b strings.Builder
	b.WriteString(")]}'\n")
	for _, text := range texts {
		inner, err := json.Marshal([]interface{}{[]interface{}{text}})
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
