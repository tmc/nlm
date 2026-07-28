package main

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/tmc/nlm/internal/notebooklm/api"
)

type fakeConversationHistoryClient struct {
	conversationIDs    []string
	messages           []api.ChatMessage
	err                error
	historyCalls       int
	lastNotebookID     string
	lastConversationID string
}

func (c *fakeConversationHistoryClient) GetConversations(context.Context, string) ([]string, error) {
	return c.conversationIDs, nil
}

func (c *fakeConversationHistoryClient) GetConversationHistory(_ context.Context, notebookID, conversationID string) ([]api.ChatMessage, error) {
	c.historyCalls++
	c.lastNotebookID = notebookID
	c.lastConversationID = conversationID
	return c.messages, c.err
}

func TestChatShowFallsBackToServerHistory(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	client := &fakeConversationHistoryClient{
		conversationIDs: []string{"abcdef12-3456-7890-abcd-ef1234567890"},
		messages: []api.ChatMessage{
			{Role: 1, Content: "question from the browser"},
			{Role: 2, Content: "answer from the server"},
		},
	}

	output := captureChatShowStdout(t, func() error {
		return chatShowWithClients("notebook", "abcdef12", chatRenderOptions{}, client, nil)
	})
	for _, want := range []string{
		"[USER]\nquestion from the browser",
		"[ASSISTANT]\nanswer from the server",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("chat show output does not contain %q:\n%s", want, output)
		}
	}
	if client.lastNotebookID != "notebook" || client.lastConversationID != "abcdef12-3456-7890-abcd-ef1234567890" {
		t.Fatalf("history request = %q %q, want notebook and full conversation ID", client.lastNotebookID, client.lastConversationID)
	}
}

func TestChatShowPrefersLocalSession(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	session := &chatSession{
		NotebookID:     "notebook",
		ConversationID: "abcdef12-3456-7890-abcd-ef1234567890",
		Messages:       []storedMessage{{Role: "assistant", Content: "local answer"}},
	}
	if err := saveChatSessionForConversation(session); err != nil {
		t.Fatal(err)
	}
	client := &fakeConversationHistoryClient{
		messages: []api.ChatMessage{{Role: 2, Content: "server answer"}},
	}

	output := captureChatShowStdout(t, func() error {
		return chatShowWithClients("notebook", "abcdef12", chatRenderOptions{}, client, nil)
	})
	if !strings.Contains(output, "local answer") {
		t.Fatalf("chat show output does not contain local answer:\n%s", output)
	}
	if strings.Contains(output, "server answer") {
		t.Fatalf("chat show output contains server answer despite local session:\n%s", output)
	}
	if client.historyCalls != 0 {
		t.Fatalf("server history calls = %d, want 0", client.historyCalls)
	}
}

func TestChatShowWithoutLocalOrServerHistoryFails(t *testing.T) {
	tests := []struct {
		name   string
		client conversationHistoryClient
	}{
		{name: "no authentication"},
		{name: "empty server history", client: &fakeConversationHistoryClient{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			err := chatShowWithClients("notebook", "missing", chatRenderOptions{}, test.client, nil)
			if err == nil {
				t.Fatal("chatShowWithClients succeeded, want error")
			}
			if !strings.Contains(err.Error(), "no local session and no server history for missing") {
				t.Fatalf("error = %q, want actionable missing-history error", err)
			}
		})
	}
}

func captureChatShowStdout(t *testing.T, f func() error) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	defer func() {
		os.Stdout = old
	}()

	runErr := f()
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	output, readErr := io.ReadAll(r)
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	if runErr != nil {
		t.Fatalf("chatShowWithClients: %v", runErr)
	}
	if readErr != nil {
		t.Fatal(readErr)
	}
	return string(output)
}
