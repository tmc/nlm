package main

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/tmc/nlm/notebooklm"
)

func TestBackfillRecoversEarlierTurns(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	const id = "12345678-1234-1234-1234-123456789abc"
	local := &chatSession{NotebookID: "nb", ConversationID: id, Messages: []storedMessage{{Role: "user", Content: "question three"}, {Role: "assistant", Content: "answer\nthree", Thinking: "local trace"}}}
	if err := saveChatSessionForConversation(local); err != nil {
		t.Fatal(err)
	}
	client := &fakeConversationHistoryClient{}
	for _, turn := range []string{"one", "two", "three"} {
		client.messages = append(client.messages, notebooklm.ChatMessage{Role: 1, Content: "question " + turn}, notebooklm.ChatMessage{Role: 2, Content: "answer " + turn})
	}
	for range 2 {
		output := captureChatShowStdout(t, func() error { return chatShowWithClients("nb", id, chatRenderOptions{Backfill: true}, client, nil) })
		if !strings.Contains(output, "answer one") || !strings.Contains(output, "answer two") {
			t.Fatal("earlier answers not rendered")
		}
		got, err := loadChatSessionForConv("nb", id)
		if err != nil {
			t.Fatal(err)
		}
		if len(got.Messages) != 6 {
			t.Fatalf("messages = %d, want 6", len(got.Messages))
		}
		if got.Messages[5].Thinking != "local trace" || got.Messages[5].Content != "answer\nthree" {
			t.Fatal("local trace or formatting lost")
		}
	}
	if _, err := os.Stat(getChatSessionPath("nb")); !os.IsNotExist(err) {
		t.Fatal("backfill changed the default session")
	}
	before, _ := os.ReadFile(getChatSessionPathForConv("nb", id))
	client.messages = nil
	if err := chatShowWithClients("nb", id, chatRenderOptions{Backfill: true}, client, nil); err == nil {
		t.Fatal("empty server history reported as successful recovery")
	}
	after, _ := os.ReadFile(getChatSessionPathForConv("nb", id))
	if string(before) != string(after) {
		t.Fatal("empty history changed local session")
	}
}

func TestMergeChatHistoryKeepsLocalTurns(t *testing.T) {
	prefix := strings.Repeat("same prefix ", 25)
	session := &chatSession{Messages: []storedMessage{
		{Role: "assistant", Content: "stream only"},
		{Role: "user", Content: "repeat"},
		{Role: "assistant", Content: prefix + "new ending", Thinking: "preserve"},
		{Role: "user", Content: "pending"},
	}}
	server := []notebooklm.ChatMessage{
		{Role: 1, Content: "repeat"}, {Role: 2, Content: prefix + "old ending"},
		{Role: 1, Content: "repeat"}, {Role: 2, Content: prefix + "new ending"},
	}
	_, added, _, _ := mergeChatHistory(session, server)
	if added != 2 {
		t.Fatalf("added = %d, want 2", added)
	}
	var got []string
	for _, message := range session.Messages {
		got = append(got, message.Content)
	}
	want := []string{"stream only", "repeat", prefix + "old ending", "repeat", prefix + "new ending", "pending"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("messages = %v", got)
	}
	if session.Messages[4].Thinking != "preserve" {
		t.Fatal("local metadata lost")
	}
	if changed, _, _, _ := mergeChatHistory(session, server); changed {
		t.Fatal("repeated backfill changed history")
	}
}

func TestBackfillSavesServerOnlyConversation(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	client := &fakeConversationHistoryClient{messages: []notebooklm.ChatMessage{{Role: 1, Content: "question"}, {Role: 2, Content: "answer"}}}
	captureChatShowStdout(t, func() error {
		return chatShowWithClients("nb", "missing", chatRenderOptions{Backfill: true}, client, nil)
	})
	session, err := loadChatSessionForConv("nb", "missing")
	if err != nil {
		t.Fatal(err)
	}
	if len(session.Messages) != 2 || client.historyCalls != 1 {
		t.Fatalf("messages = %d, history calls = %d", len(session.Messages), client.historyCalls)
	}
}
