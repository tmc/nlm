package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/notebooklm/api"
)

func TestParseSourceSelectionArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantPos []string
		wantSel selectorOptions
		wantErr string
	}{
		{
			name:    "selectors without positional source ids",
			args:    []string{"nb", "--source-match", "^spec/"},
			wantPos: []string{"nb"},
			wantSel: selectorOptions{SourceMatch: "^spec/"},
		},
		{
			name:    "positional source ids still work",
			args:    []string{"nb", "src-1", "src-2"},
			wantPos: []string{"nb", "src-1", "src-2"},
		},
		{
			name:    "missing source ids and selectors",
			args:    []string{"nb"},
			wantErr: "missing source ids or selectors",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotPos, err := parseSourceSelectionArgsWithOptions(tt.args, globalOptions{})
			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr {
					t.Fatalf("parseSourceSelectionArgs(%q) error = %v, want %q", tt.args, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseSourceSelectionArgs(%q) error = %v", tt.args, err)
			}
			if got.Selectors != tt.wantSel {
				t.Fatalf("parseSourceSelectionArgs(%q) selectors = %+v, want %+v", tt.args, got.Selectors, tt.wantSel)
			}
			if len(gotPos) != len(tt.wantPos) {
				t.Fatalf("parseSourceSelectionArgs(%q) positional = %q, want %q", tt.args, gotPos, tt.wantPos)
			}
			for i := range gotPos {
				if gotPos[i] != tt.wantPos[i] {
					t.Fatalf("parseSourceSelectionArgs(%q) positional = %q, want %q", tt.args, gotPos, tt.wantPos)
				}
			}
		})
	}
}

func TestParseGenerateChatArgs(t *testing.T) {
	got, gotPos, err := parseGenerateChatArgsWithOptions([]string{
		"nb",
		"why",
		"--conversation", "conv-1",
		"--thinking",
		"--source-match", "^spec/",
		"now",
	}, globalOptions{})
	if err != nil {
		t.Fatalf("parseGenerateChatArgs error = %v", err)
	}
	if got.ConversationID != "conv-1" || !got.Render.ShowThinking || got.Selectors.SourceMatch != "^spec/" {
		t.Fatalf("parseGenerateChatArgs opts = %+v", got)
	}
	wantPos := []string{"nb", "why", "now"}
	if len(gotPos) != len(wantPos) {
		t.Fatalf("parseGenerateChatArgs positional = %q, want %q", gotPos, wantPos)
	}
	for i := range gotPos {
		if gotPos[i] != wantPos[i] {
			t.Fatalf("parseGenerateChatArgs positional = %q, want %q", gotPos, wantPos)
		}
	}
}

func TestParseGenerateChatArgsPromptFile(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "long flag",
			args: []string{"--prompt-file", "prompt.txt", "nb"},
			want: "prompt.txt",
		},
		{
			name: "short flag",
			args: []string{"nb", "-f", "-"},
			want: "-",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotPos, err := parseGenerateChatArgsWithOptions(tt.args, globalOptions{})
			if err != nil {
				t.Fatalf("parseGenerateChatArgs error = %v", err)
			}
			if got.PromptFile != tt.want {
				t.Fatalf("prompt file = %q, want %q", got.PromptFile, tt.want)
			}
			if len(gotPos) != 1 || gotPos[0] != "nb" {
				t.Fatalf("positional = %q, want [nb]", gotPos)
			}
		})
	}
}

func TestParseChatArgs(t *testing.T) {
	got, gotPos, err := parseChatArgsWithOptions([]string{
		"nb",
		"--prompt-file", "prompt.txt",
		"--history",
		"--citations", "tail",
		"--source-ids", "a,b",
	}, globalOptions{})
	if err != nil {
		t.Fatalf("parseChatArgs error = %v", err)
	}
	if got.PromptFile != "prompt.txt" || !got.ShowHistory || got.Render.CitationMode != "tail" || got.Selectors.SourceIDs != "a,b" {
		t.Fatalf("parseChatArgs opts = %+v", got)
	}
	if len(gotPos) != 1 || gotPos[0] != "nb" {
		t.Fatalf("parseChatArgs positional = %q, want [nb]", gotPos)
	}
}

func TestParseGenerateReportArgs(t *testing.T) {
	got, gotPos, err := parseGenerateReportArgsWithOptions([]string{
		"nb",
		"--sections", "3",
		"--prompt", "# {topic}",
		"--source-match", "^guide/",
	}, globalOptions{})
	if err != nil {
		t.Fatalf("parseGenerateReportArgs error = %v", err)
	}
	if got.Sections != 3 || got.Prompt != "# {topic}" || got.Selectors.SourceMatch != "^guide/" {
		t.Fatalf("parseGenerateReportArgs opts = %+v", got)
	}
	if len(gotPos) != 1 || gotPos[0] != "nb" {
		t.Fatalf("parseGenerateReportArgs positional = %q, want [nb]", gotPos)
	}
}

func TestParseCreateReportArgsUsesGlobalSelectors(t *testing.T) {
	inv, err := parseInvocation([]string{"--source-match", "^spec/", "create-report", "nb", "brief"}, nil, nil, os.Stderr)
	if err != nil {
		t.Fatalf("parseInvocation: %v", err)
	}
	got, gotPos, err := parseCreateReportArgsWithOptions(inv.args, inv.globals)
	if err != nil {
		t.Fatalf("parseCreateReportArgs: %v", err)
	}
	if got.Selectors.SourceMatch != "^spec/" {
		t.Fatalf("create-report selectors = %+v, want source-match from globals", got.Selectors)
	}
	if strings.Join(gotPos, ",") != "nb,brief" {
		t.Fatalf("create-report positional = %q, want [nb brief]", gotPos)
	}
}

func TestParseCreateReportArgsLocalSelectors(t *testing.T) {
	got, gotPos, err := parseCreateReportArgsWithOptions([]string{"nb", "brief", "--source-ids", "src-1,src-2", "desc"}, globalOptions{})
	if err != nil {
		t.Fatalf("parseCreateReportArgs: %v", err)
	}
	if got.Selectors.SourceIDs != "src-1,src-2" {
		t.Fatalf("create-report selectors = %+v, want source ids", got.Selectors)
	}
	if strings.Join(gotPos, ",") != "nb,brief,desc" {
		t.Fatalf("create-report positional = %q, want [nb brief desc]", gotPos)
	}
}

func TestParseChatShowArgsResolveCitationsCompatibility(t *testing.T) {
	got, gotPos, err := parseChatShowArgsWithOptions([]string{"nb", "conv", "--resolve-citations"}, globalOptions{})
	if err != nil {
		t.Fatalf("parseChatShowArgs: %v", err)
	}
	if !got.ResolveCitations {
		t.Fatalf("chat show resolve citations = false, want true")
	}
	if strings.Join(gotPos, ",") != "nb,conv" {
		t.Fatalf("chat show positional = %q, want [nb conv]", gotPos)
	}
}

func TestParseChatShowIncludeFollowUps(t *testing.T) {
	opts, positional, err := parseChatShowArgsWithOptions([]string{
		"--format=html", "--include-follow-ups", "notebook", "conversation",
	}, globalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !opts.IncludeFollowUps {
		t.Fatal("IncludeFollowUps = false, want true")
	}
	if got, want := strings.Join(positional, " "), "notebook conversation"; got != want {
		t.Fatalf("positional = %q, want %q", got, want)
	}
}

func TestParseChatShowHTMLOutput(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantOut string
		wantErr bool
	}{
		{
			name: "default cache",
			args: []string{"--format=html", "notebook", "conversation"},
		},
		{
			name:    "explicit file",
			args:    []string{"--format=html", "--out=conversation.html", "notebook", "conversation"},
			wantOut: "conversation.html",
		},
		{
			name:    "stdout",
			args:    []string{"--format=html", "--out=-", "notebook", "conversation"},
			wantOut: "-",
		},
		{
			name:    "cannot open stdout",
			args:    []string{"--format=html", "--out=-", "--open", "notebook", "conversation"},
			wantOut: "-",
			wantErr: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			opts, _, err := parseChatShowArgsWithOptions(test.args, globalOptions{})
			if (err != nil) != test.wantErr {
				t.Fatalf("error = %v, want error %v", err, test.wantErr)
			}
			if opts.OutFile != test.wantOut {
				t.Fatalf("OutFile = %q, want %q", opts.OutFile, test.wantOut)
			}
		})
	}
}

func TestParseChatShowNotebook(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantFormat string
		wantErr    bool
	}{
		{name: "defaults to html", args: []string{"notebook"}, wantFormat: "html"},
		{name: "explicit html", args: []string{"--format=html", "notebook"}, wantFormat: "html"},
		{name: "text rejected", args: []string{"--format=text", "notebook"}, wantFormat: "text", wantErr: true},
		{name: "backfill needs conversation", args: []string{"--backfill", "notebook"}, wantFormat: "html", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			opts, positional, err := parseChatShowArgsWithOptions(test.args, globalOptions{})
			if (err != nil) != test.wantErr {
				t.Fatalf("error = %v, want error %v", err, test.wantErr)
			}
			if opts.Format != test.wantFormat {
				t.Fatalf("format = %q, want %q", opts.Format, test.wantFormat)
			}
			if !test.wantErr && (len(positional) != 1 || positional[0] != "notebook") {
				t.Fatalf("positional = %q, want [notebook]", positional)
			}
		})
	}
}

func TestParseChatShowBackfill(t *testing.T) {
	opts, positional, err := parseChatShowArgsWithOptions([]string{
		"--backfill", "notebook", "conversation",
	}, globalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !opts.Backfill {
		t.Fatal("Backfill = false, want true")
	}
	if got, want := strings.Join(positional, " "), "notebook conversation"; got != want {
		t.Fatalf("positional = %q, want %q", got, want)
	}
}

func TestSaveChatSessionWritesConversationFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	session := &ChatSession{
		NotebookID:     "nb",
		ConversationID: "12345678-1234-1234-1234-123456789abc",
		Messages:       []ChatMessage{{Role: "user", Content: "hello"}},
	}
	if err := saveChatSession(session); err != nil {
		t.Fatalf("saveChatSession: %v", err)
	}
	if _, err := os.Stat(getChatSessionPath("nb")); err != nil {
		t.Fatalf("legacy session file missing: %v", err)
	}
	if _, err := os.Stat(getChatSessionPathForConv("nb", session.ConversationID)); err != nil {
		t.Fatalf("conversation session file missing: %v", err)
	}
}

func TestChatSessionRichRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	rich := testProtoRichDocument("hello")
	session := &ChatSession{
		NotebookID:     "nb",
		ConversationID: "12345678-1234-1234-1234-123456789abc",
		Messages: []ChatMessage{{
			Role: "assistant", Content: "hello", Rich: rich,
		}},
	}
	if err := saveChatSession(session); err != nil {
		t.Fatal(err)
	}
	got, err := loadChatSessionByConversation("nb", session.ConversationID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Messages) != 1 || got.Messages[0].Rich == nil ||
		len(got.Messages[0].Rich.GetBody().GetBlocks()) != 1 {
		t.Fatalf("loaded messages = %+v", got.Messages)
	}
	doc := chatDocument{Messages: []chatDocMessage{{
		Role: "assistant", Content: got.Messages[0].Content,
		Rich: richDocumentFromProto(got.Messages[0].Rich),
	}}}
	if html := renderToString(t, doc, chatRenderContext{}); !strings.Contains(html, "<p>hello</p>") {
		t.Fatalf("rich round-trip did not take tree renderer:\n%s", html)
	}
}

func TestSaveChatSessionForConversationDoesNotReplaceDefault(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	active := &ChatSession{
		NotebookID:     "nb",
		ConversationID: "aaaaaaaa-1234-1234-1234-123456789abc",
		Messages:       []ChatMessage{{Role: "assistant", Content: "active"}},
	}
	if err := saveChatSession(active); err != nil {
		t.Fatal(err)
	}
	backfilled := &ChatSession{
		NotebookID:     "nb",
		ConversationID: "bbbbbbbb-1234-1234-1234-123456789abc",
		Messages:       []ChatMessage{{Role: "assistant", Content: "older"}},
	}
	if err := saveChatSessionForConversation(backfilled); err != nil {
		t.Fatal(err)
	}
	got, err := loadChatSession("nb")
	if err != nil {
		t.Fatal(err)
	}
	if got.ConversationID != active.ConversationID || got.Messages[0].Content != "active" {
		t.Fatalf("default session replaced: %+v", got)
	}
	older, err := loadChatSessionForConv("nb", backfilled.ConversationID)
	if err != nil {
		t.Fatal(err)
	}
	if older.Messages[0].Content != "older" {
		t.Fatalf("conversation session = %+v", older)
	}
}

func TestMergeChatHistory(t *testing.T) {
	key := citationContentKey("answer")
	serverRich := testProtoRichDocument("answer")
	localRich := testProtoRichDocument("local")
	serverCitations := []api.Citation{{SourceIndex: 1, SourceID: "server"}}
	localCitations := []api.Citation{{SourceIndex: 2, SourceID: "local"}}
	tests := []struct {
		name              string
		message           ChatMessage
		rich              map[string]*pb.RichDocument
		citations         map[string][]api.Citation
		wantChanged       bool
		wantRich, wantCit int
	}{
		{
			name: "fills gaps",
			message: ChatMessage{
				Role: "assistant", Content: "answer", Thinking: "keep me",
			},
			rich:        map[string]*pb.RichDocument{key: serverRich},
			citations:   map[string][]api.Citation{key: serverCitations},
			wantChanged: true, wantRich: 1, wantCit: 1,
		},
		{
			name: "preserves local data",
			message: ChatMessage{
				Role: "assistant", Content: "answer", Thinking: "keep me",
				Rich: localRich, Citations: localCitations,
			},
			rich:      map[string]*pb.RichDocument{key: serverRich},
			citations: map[string][]api.Citation{key: serverCitations},
		},
		{
			name: "server flat leaves local flat",
			message: ChatMessage{
				Role: "assistant", Content: "answer", Thinking: "keep me",
			},
			rich:      map[string]*pb.RichDocument{},
			citations: map[string][]api.Citation{},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			session := &ChatSession{Messages: []ChatMessage{test.message}}
			changed, richCount, citationCount := mergeChatHistory(session, test.rich, test.citations)
			if changed != test.wantChanged || richCount != test.wantRich || citationCount != test.wantCit {
				t.Fatalf("merge = %v,%d,%d, want %v,%d,%d", changed, richCount, citationCount, test.wantChanged, test.wantRich, test.wantCit)
			}
			got := session.Messages[0]
			if got.Thinking != "keep me" {
				t.Fatalf("Thinking = %q, want preserved", got.Thinking)
			}
			if test.name == "preserves local data" &&
				(got.Rich != localRich || got.Citations[0].SourceID != "local") {
				t.Fatalf("local data overwritten: %+v", got)
			}
		})
	}
}

func TestLoadChatSessionByConversationPrefixFallsBackToLegacy(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	session := `{
  "notebook_id": "nb",
  "conversation_id": "abcdef12-3456-7890-abcd-ef1234567890",
  "messages": [{"role": "assistant", "content": "smoke ok"}]
}`
	path := filepath.Join(home, ".nlm")
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "chat-nb.json"), []byte(session), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := loadChatSessionByConversation("nb", "abcdef12")
	if err != nil {
		t.Fatalf("loadChatSessionByConversation: %v", err)
	}
	if got.ConversationID != "abcdef12-3456-7890-abcd-ef1234567890" {
		t.Fatalf("conversation = %q", got.ConversationID)
	}
	if len(got.Messages) != 1 || !strings.Contains(got.Messages[0].Content, "smoke") {
		t.Fatalf("messages = %+v", got.Messages)
	}
}
