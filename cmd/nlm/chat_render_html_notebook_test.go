package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/tmc/nlm/internal/notebooklm/api"
)

func TestLoadNotebookSessions(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	now := time.Date(2026, 7, 26, 10, 0, 0, 0, time.UTC)
	sessions := []struct {
		name    string
		session ChatSession
	}{
		{
			name: "chat-nb-old.json",
			session: ChatSession{
				NotebookID:     "nb",
				ConversationID: "old",
				Messages:       []ChatMessage{{Role: "user", Content: "old"}},
				CreatedAt:      now.Add(-3 * time.Hour),
			},
		},
		{
			name: "chats/nb/new.json",
			session: ChatSession{
				NotebookID:     "nb",
				ConversationID: "new",
				Messages:       []ChatMessage{{Role: "user", Content: "new"}},
				UpdatedAt:      now,
			},
		},
		{
			name: "chat-nb-new.json",
			session: ChatSession{
				NotebookID:     "nb",
				ConversationID: "new",
				Messages:       []ChatMessage{{Role: "user", Content: "stale duplicate"}},
				UpdatedAt:      now.Add(-time.Hour),
			},
		},
		{
			name: "chat-other.json",
			session: ChatSession{
				NotebookID:     "other",
				ConversationID: "other",
				Messages:       []ChatMessage{{Role: "user", Content: "other notebook"}},
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

func TestRenderNotebookHTML(t *testing.T) {
	now := time.Date(2026, 7, 26, 10, 0, 0, 0, time.UTC)
	docs := []notebookChatDocument{
		{
			Document: notebookTestDocument("aaaaaaaa-1111", `Newest </script><script>alert(1)</script>`, "Alpha [1]"),
			Updated:  now,
		},
		{
			Document: notebookTestDocument("bbbbbbbb-2222", "Older query", "Beta [1]"),
			Updated:  now.Add(-time.Hour),
		},
	}
	var out strings.Builder
	if err := renderNotebookHTML(&out, docs, chatRenderContext{}); err != nil {
		t.Fatal(err)
	}

	payload := decodeNotebookHTMLPayload(t, out.String())
	if len(payload.Conversations) != 2 {
		t.Fatalf("conversations = %d, want 2", len(payload.Conversations))
	}
	if got := payload.Conversations[0].ID; got != "aaaaaaaa-1111" {
		t.Fatalf("default conversation = %q, want newest", got)
	}
	for _, conversation := range payload.Conversations {
		want := `href="#cite-` + conversation.Namespace + `-1-1"`
		if !strings.Contains(conversation.HTML, want) {
			t.Errorf("%s: missing namespaced link %q", conversation.ID, want)
		}
		if strings.Contains(conversation.HTML, `href="#cite-1-1"`) {
			t.Errorf("%s: contains unnamespaced citation link", conversation.ID)
		}
		wantIDCode := `return "cite-` + conversation.Namespace + `-" + msgIdx + "-" + idx`
		if !strings.Contains(conversation.HTML, wantIDCode) {
			t.Errorf("%s: citation target builder is not namespaced", conversation.ID)
		}
	}
	if strings.Count(out.String(), `frame.srcdoc = conversation.html`) != 1 {
		t.Error("notebook does not swap buffered pages through one frame")
	}
	if strings.Contains(out.String(), `</script><script>alert(1)</script>`) {
		t.Error("conversation title escaped the notebook data payload")
	}
	for _, token := range []string{
		`id="conversation-search"`,
		`id="sidebar-toggle"`,
		`search.addEventListener("input"`,
		`sidebar.classList.toggle("open")`,
		`sidebar:not(.open) .list`,
		`@media (max-width: 860px)`,
		`button.addEventListener("click"`,
		`if (data.conversations.length) select(0)`,
	} {
		if !strings.Contains(out.String(), token) {
			t.Errorf("notebook output missing %q", token)
		}
	}
}

func TestNamespaceChatHTMLDoesNotChangeInput(t *testing.T) {
	const input = `<a href="#cite-1-2">2</a>
function citeId(msgIdx, idx) { return "cite-" + msgIdx + "-" + idx; }`
	got, err := namespaceChatHTML(input, "conv-abc")
	if err != nil {
		t.Fatal(err)
	}
	if input != `<a href="#cite-1-2">2</a>
function citeId(msgIdx, idx) { return "cite-" + msgIdx + "-" + idx; }` {
		t.Fatal("namespaceChatHTML changed its input")
	}
	if !strings.Contains(got, `href="#cite-conv-abc-1-2"`) {
		t.Fatalf("output = %q", got)
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

func notebookTestDocument(id, question, answer string) chatDocument {
	return chatDocument{
		NotebookID:     "nb",
		ConversationID: id,
		Messages: []chatDocMessage{
			{Role: "user", Content: question},
			{
				Role:    "assistant",
				Content: answer,
				Citations: []api.Citation{{
					SourceIndex: 1,
					SourceID:    "source-" + id,
					Title:       "Source",
					StartChar:   0,
					EndChar:     5,
				}},
			},
		},
	}
}

var notebookDataPattern = regexp.MustCompile(`(?s)<script id="notebook-data" type="application/json">(.*?)</script>`)

func decodeNotebookHTMLPayload(t *testing.T, page string) notebookHTMLPayload {
	t.Helper()
	match := notebookDataPattern.FindStringSubmatch(page)
	if match == nil {
		t.Fatal("notebook data payload not found")
	}
	var payload notebookHTMLPayload
	if err := json.Unmarshal([]byte(match[1]), &payload); err != nil {
		t.Fatalf("decode notebook payload: %v", err)
	}
	return payload
}
