package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadNotebookSessions(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	now := time.Date(2026, 7, 26, 10, 0, 0, 0, time.UTC)
	sessions := []struct {
		name    string
		session chatSession
	}{
		{
			name: "chat-nb-old.json",
			session: chatSession{
				NotebookID:     "nb",
				ConversationID: "old",
				Messages:       []storedMessage{{Role: "user", Content: "old"}},
				CreatedAt:      now.Add(-3 * time.Hour),
			},
		},
		{
			name: "chats/nb/new.json",
			session: chatSession{
				NotebookID:     "nb",
				ConversationID: "new",
				Messages:       []storedMessage{{Role: "user", Content: "new"}},
				UpdatedAt:      now,
			},
		},
		{
			name: "chat-nb-new.json",
			session: chatSession{
				NotebookID:     "nb",
				ConversationID: "new",
				Messages:       []storedMessage{{Role: "user", Content: "stale duplicate"}},
				UpdatedAt:      now.Add(-time.Hour),
			},
		},
		{
			name: "chat-other.json",
			session: chatSession{
				NotebookID:     "other",
				ConversationID: "other",
				Messages:       []storedMessage{{Role: "user", Content: "other notebook"}},
			},
		},
	}
	for _, item := range sessions {
		path := filepath.Join(home, ".nlm", item.name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		data, err := json.Marshal(item.session)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	got, err := loadNotebookSessions("nb")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("sessions = %d, want 2", len(got))
	}
	if got[0].ConversationID != "new" || got[0].Messages[0].Content != "new" {
		t.Fatalf("first session = %#v, want newest nested copy", got[0])
	}
	if got[1].ConversationID != "old" {
		t.Fatalf("second conversation = %q, want old", got[1].ConversationID)
	}
}

func TestNotebookHTMLDestination(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	cache, err := os.UserCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		outFile string
		want    string
	}{
		{name: "stdout", outFile: "-", want: ""},
		{name: "explicit", outFile: "all.html", want: "all.html"},
		{name: "cache", want: filepath.Join(cache, "nlm", "render", "nb", "notebook.html")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := notebookHTMLDestination("nb", test.outFile)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("destination = %q, want %q", got, test.want)
			}
		})
	}
}
