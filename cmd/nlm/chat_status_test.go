package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestResolveChatStatus(t *testing.T) {
	path := filepath.Join(t.TempDir(), "chat.json")
	now := time.Now()
	for _, tt := range []struct {
		name, status string
		pid          int
		want         string
	}{
		{"complete", chatStatusComplete, 0, ""},
		{"error", chatStatusError, 0, "failed"},
		{"incomplete", chatStatusIncomplete, 0, "truncated"},
		{"abandoned", chatStatusRunning, 0, "interrupted"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveChatStatus(tt.status, now, tt.pid, path, now); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
	if err := os.WriteFile(chatPartialPath(path), []byte("{}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := resolveChatStatus(chatStatusRunning, now, 0, path, now); got != "generating" {
		t.Fatal(got)
	}
	old := now.Add(-2 * chatStaleAfter)
	if err := os.Chtimes(chatPartialPath(path), old, old); err != nil {
		t.Fatal(err)
	}
	if got := resolveChatStatus(chatStatusRunning, now, 0, path, now); got != "interrupted" {
		t.Fatal(got)
	}
}

func TestChatSummaryStatusAndTitle(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	if err := beginGeneratedChatTurn("nb", "conv", nil, "first question"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		summaries, err := listLocalChatSessionSummaries("nb")
		if err != nil {
			t.Fatal(err)
		}
		if len(summaries) != 2 {
			t.Fatalf("summaries=%d", len(summaries))
		}
		for _, s := range summaries {
			if s.Title != "first question" || s.Status != chatStatusRunning || s.WriterPID != os.Getpid() || s.StatusAt.IsZero() {
				t.Fatalf("summary=%+v", s)
			}
		}
	}
}
