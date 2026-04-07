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
	err := c.parseChatResponseChunked(strings.NewReader(stream), func(chunk ChatChunk) bool {
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
