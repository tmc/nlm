package main

import (
	"os"
	"reflect"
	"testing"

	"github.com/tmc/nlm/notebooklm"
)

func TestSaveGeneratedChatTurn(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	const id = "12345678-1234-1234-1234-123456789abc"
	for _, turn := range []string{"one", "two", "three"} {
		if err := saveGeneratedChatTurn("nb", id, nil, "question "+turn, chatResult{Answer: "answer " + turn, Thinking: "trace " + turn}); err != nil {
			t.Fatal(err)
		}
	}
	session, err := loadChatSessionForConv("nb", id)
	if err != nil {
		t.Fatal(err)
	}
	if len(session.Messages) != 6 {
		t.Fatalf("messages = %d, want 6", len(session.Messages))
	}
	before := append([]storedMessage(nil), session.Messages...)
	if session.Messages[1].Thinking != "trace one" || session.Messages[5].Content != "answer three" {
		t.Fatal("earlier exchange lost")
	}
	created := session.CreatedAt
	if err := saveGeneratedChatTurn("nb", id, nil, "unfinished", chatResult{Answer: "partial", Incomplete: true}); err != nil {
		t.Fatal(err)
	}
	session, err = loadChatSessionForConv("nb", id)
	if err != nil {
		t.Fatal(err)
	}
	if len(session.Messages) != 7 || !reflect.DeepEqual(session.Messages[:6], before) || !session.CreatedAt.Equal(created) {
		t.Fatal("incomplete turn replaced history")
	}
	client := &fakeConversationHistoryClient{}
	if got := resolveConversationID(client, "nb", id[:8]); got != id {
		t.Fatalf("resolved ID = %q", got)
	}
	// An unreadable session must not be silently replaced by a fresh pair.
	path := getChatSessionPathForConv("nb", id)
	if err := os.WriteFile(path, []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := saveGeneratedChatTurn("nb", id, nil, "new", chatResult{Answer: "new"}); err == nil {
		t.Fatal("corrupt session overwritten")
	}
	data, _ := os.ReadFile(path)
	if string(data) != "broken" {
		t.Fatal("corrupt session changed")
	}
}

func TestBeginGeneratedChatTurnWritesRunningSession(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := beginGeneratedChatTurn("nb", "conv", nil, "question"); err != nil {
		t.Fatal(err)
	}
	session, err := loadChatSessionForConv("nb", "conv")
	if err != nil {
		t.Fatal(err)
	}
	if session.Status != chatStatusRunning {
		t.Fatalf("status = %q, want %q", session.Status, chatStatusRunning)
	}
	if len(session.Messages) != 1 || session.Messages[0].Content != "question" {
		t.Fatalf("messages = %+v, want pending user turn", session.Messages)
	}
	if err := saveGeneratedChatTurn("nb", "conv", nil, "question", chatResult{Answer: "answer"}); err != nil {
		t.Fatal(err)
	}
	session, err = loadChatSessionForConv("nb", "conv")
	if err != nil {
		t.Fatal(err)
	}
	if session.Status != chatStatusComplete || len(session.Messages) != 2 {
		t.Fatalf("completed session = status %q, messages %d", session.Status, len(session.Messages))
	}
}

func TestSaveGeneratedChatTurnFromServer(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	history := []notebooklm.ChatMessage{{Role: 2, Content: "old answer"}, {Role: 1, Content: "old question"}}
	if err := saveGeneratedChatTurn("nb", "conv", history, "new question", chatResult{Answer: "new answer"}); err != nil {
		t.Fatal(err)
	}
	session, err := loadChatSessionForConv("nb", "conv")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, m := range session.Messages {
		got = append(got, m.Content)
	}
	want := []string{"old question", "old answer", "new question", "new answer"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("messages = %v", got)
	}
}
