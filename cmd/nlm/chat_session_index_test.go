package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeTestSession stores a session under a temporary HOME and returns its path.
func writeTestSession(t *testing.T, session chatSession) string {
	t.Helper()
	data, err := json.Marshal(session)
	if err != nil {
		t.Fatal(err)
	}
	path := getChatSessionPathForConv(session.NotebookID, session.ConversationID)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func testSession(notebookID, conversationID string, messages int) chatSession {
	s := chatSession{
		NotebookID:     notebookID,
		ConversationID: conversationID,
		UpdatedAt:      time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC),
	}
	for i := 0; i < messages; i++ {
		s.Messages = append(s.Messages, storedMessage{Role: "user", Content: "hello"})
	}
	return s
}

func TestReadChatSessionSummary(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	path := writeTestSession(t, testSession("nb-1", "conv-1111-2222", 5))

	got, err := readChatSessionSummary(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.NotebookID != "nb-1" || got.ConversationID != "conv-1111-2222" || got.MessageCount != 5 {
		t.Fatalf("summary = %+v, want nb-1/conv-1111-2222/5", got)
	}
	if !got.UpdatedAt.Equal(time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)) {
		t.Fatalf("UpdatedAt = %v", got.UpdatedAt)
	}
}

// TestListLocalChatSessionSummariesFiltersByFilename is the point of the
// index: a notebook's sessions must be found without reading the sessions of
// every other notebook.
func TestListLocalChatSessionSummariesFiltersByFilename(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))
	writeTestSession(t, testSession("nb-1", "conv-a", 2))
	writeTestSession(t, testSession("nb-1", "conv-b", 4))
	other := writeTestSession(t, testSession("nb-2", "conv-c", 6))

	// A file that must never be opened by the nb-1 listing: make it
	// unparseable, so reading it would show up as a missing summary.
	if err := os.WriteFile(other, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := listLocalChatSessionSummaries("nb-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("summaries = %+v, want 2", got)
	}
	counts := map[string]int{}
	for _, s := range got {
		counts[s.ConversationID] = s.MessageCount
	}
	if counts["conv-a"] != 2 || counts["conv-b"] != 4 {
		t.Fatalf("counts = %v, want conv-a=2 conv-b=4", counts)
	}
}

// TestListLocalChatSessionSummariesCacheTracksEdits covers the stat guard: a
// rewritten session must not be reported from the stale cached entry.
func TestListLocalChatSessionSummariesCacheTracksEdits(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))
	writeTestSession(t, testSession("nb-1", "conv-a", 2))

	if got, _ := listLocalChatSessionSummaries("nb-1"); len(got) != 1 || got[0].MessageCount != 2 {
		t.Fatalf("first listing = %+v, want one summary with 2 messages", got)
	}
	path := writeTestSession(t, testSession("nb-1", "conv-a", 7))
	// Make the change visible to a modification-time comparison even on a
	// coarse-grained clock.
	future := time.Now().Add(time.Second)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatal(err)
	}
	got, err := listLocalChatSessionSummaries("nb-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].MessageCount != 7 {
		t.Fatalf("second listing = %+v, want one summary with 7 messages", got)
	}
}

func TestFindLocalChatSession(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	writeTestSession(t, testSession("nb-1", "aaaaaaaa-1111-2222-3333-444444444444", 3))
	writeTestSession(t, testSession("nb-1", "bbbbbbbb-1111-2222-3333-444444444444", 1))

	for _, arg := range []string{"aaaaaaaa-1111-2222-3333-444444444444", "aaaaaaaa", "aaa"} {
		session, err := findLocalChatSession("nb-1", arg)
		if err != nil {
			t.Fatalf("findLocalChatSession(%q) error = %v", arg, err)
		}
		if len(session.Messages) != 3 {
			t.Fatalf("findLocalChatSession(%q) messages = %d, want 3", arg, len(session.Messages))
		}
	}
	if _, err := findLocalChatSession("nb-1", "cccc"); !os.IsNotExist(err) {
		t.Fatalf("findLocalChatSession on unknown conversation = %v, want not-exist", err)
	}
	if _, err := findLocalChatSession("nb-2", "aaaaaaaa"); !os.IsNotExist(err) {
		t.Fatal("findLocalChatSession crossed notebooks")
	}
}
