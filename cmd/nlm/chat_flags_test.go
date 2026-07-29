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
			command, ok := lookupCommand("source-guide")
			if !ok {
				t.Fatal("source-guide command not found")
			}
			parsed, err := parseCommandSpec(command.spec, command.surfaceSpec, tt.args, globalOptions{})
			var got sourceGuideArgs
			if err == nil {
				got, err = decodeSourceGuideArgs(parsed)
			}
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
				t.Fatalf("parse source-guide %q selectors = %+v, want %+v", tt.args, got.Selectors, tt.wantSel)
			}
			gotPos := append([]string{got.NotebookID}, got.SourceIDs...)
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
	parsed := parseChatCommandForTest(t, "generate-chat", []string{
		"nb",
		"why",
		"--conversation", "conv-1",
		"--thinking",
		"--source-match", "^spec/",
		"now",
	}, globalOptions{})
	got, err := decodeGenerateChatArgs(parsed)
	if err != nil {
		t.Fatalf("parseGenerateChatArgs error = %v", err)
	}
	if got.Options.ConversationID != "conv-1" ||
		!got.Options.Render.ShowThinking ||
		got.Options.Selectors.SourceMatch != "^spec/" {
		t.Fatalf("parseGenerateChatArgs opts = %+v", got)
	}
	if got.NotebookID != "nb" || got.Prompt != "why now" {
		t.Fatalf("parseGenerateChatArgs arguments = %+v", got)
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
			parsed := parseChatCommandForTest(t, "generate-chat", tt.args, globalOptions{})
			got, err := decodeGenerateChatArgs(parsed)
			if err != nil {
				t.Fatalf("parseGenerateChatArgs error = %v", err)
			}
			if got.Options.PromptFile != tt.want {
				t.Fatalf("prompt file = %q, want %q", got.Options.PromptFile, tt.want)
			}
			if got.NotebookID != "nb" {
				t.Fatalf("notebook = %q, want nb", got.NotebookID)
			}
		})
	}
}

func TestParseChatArgs(t *testing.T) {
	parsed := parseChatCommandForTest(t, "chat", []string{
		"nb",
		"--prompt-file", "prompt.txt",
		"--history",
		"--citations", "tail",
		"--source-ids", "a,b",
	}, globalOptions{})
	got, err := decodeChatArgs(parsed)
	if err != nil {
		t.Fatalf("parseChatArgs error = %v", err)
	}
	if got.Options.PromptFile != "prompt.txt" ||
		!got.Options.ShowHistory ||
		got.Options.Render.CitationMode != "tail" ||
		got.Options.Selectors.SourceIDs != "a,b" {
		t.Fatalf("parseChatArgs opts = %+v", got)
	}
	if got.NotebookID != "nb" || len(got.Rest) != 0 {
		t.Fatalf("parseChatArgs arguments = %+v", got)
	}
}

func TestParseGenerateReportArgs(t *testing.T) {
	parsed := parseChatCommandForTest(t, "generate-report", []string{
		"nb",
		"--sections", "3",
		"--prompt", "# {topic}",
		"--source-match", "^guide/",
	}, globalOptions{})
	got, err := decodeGenerateReportArgs(parsed)
	if err != nil {
		t.Fatalf("parseGenerateReportArgs error = %v", err)
	}
	if got.Options.Sections != 3 ||
		got.Options.Prompt != "# {topic}" ||
		got.Options.Selectors.SourceMatch != "^guide/" {
		t.Fatalf("parseGenerateReportArgs opts = %+v", got)
	}
	if got.NotebookID != "nb" {
		t.Fatalf("notebook = %q, want nb", got.NotebookID)
	}
}

func TestParseCreateReportArgsUsesGlobalSelectors(t *testing.T) {
	inv, err := parseInvocation([]string{"--source-match", "^spec/", "create-report", "nb", "brief"}, nil, nil, os.Stderr)
	if err != nil {
		t.Fatalf("parseInvocation: %v", err)
	}
	parsed := parseChatCommandForTest(t, "create-report", inv.args, inv.globals)
	got, err := decodeCreateReportArgs(parsed)
	if err != nil {
		t.Fatalf("parseCreateReportArgs: %v", err)
	}
	if got.Options.Selectors.SourceMatch != "^spec/" {
		t.Fatalf("create-report selectors = %+v, want source-match from globals", got.Options.Selectors)
	}
	if got.NotebookID != "nb" || got.ReportType != "brief" || len(got.Extra) != 0 {
		t.Fatalf("create-report arguments = %+v", got)
	}
}

func TestParseCreateReportArgsLocalSelectors(t *testing.T) {
	parsed := parseChatCommandForTest(t, "create-report", []string{
		"nb", "brief", "--source-ids", "src-1,src-2", "desc",
	}, globalOptions{})
	got, err := decodeCreateReportArgs(parsed)
	if err != nil {
		t.Fatalf("parseCreateReportArgs: %v", err)
	}
	if got.Options.Selectors.SourceIDs != "src-1,src-2" {
		t.Fatalf("create-report selectors = %+v, want source ids", got.Options.Selectors)
	}
	if got.NotebookID != "nb" || got.ReportType != "brief" || strings.Join(got.Extra, ",") != "desc" {
		t.Fatalf("create-report arguments = %+v", got)
	}
}

func TestParseChatShowArgsResolveCitationsCompatibility(t *testing.T) {
	parsed := parseChatCommandForTest(t, "chat show", []string{
		"nb", "conv", "--resolve-citations",
	}, globalOptions{})
	got, err := decodeChatShowArgs(parsed)
	if err != nil {
		t.Fatalf("parseChatShowArgs: %v", err)
	}
	if !got.Options.ResolveCitations {
		t.Fatalf("chat show resolve citations = false, want true")
	}
	if got.NotebookID != "nb" || got.ConversationID != "conv" {
		t.Fatalf("chat show arguments = %+v", got)
	}
}

func TestParseChatShowIncludeFollowUps(t *testing.T) {
	parsed := parseChatCommandForTest(t, "chat show", []string{
		"--format=html", "--include-follow-ups", "notebook", "conversation",
	}, globalOptions{})
	args, err := decodeChatShowArgs(parsed)
	if err != nil {
		t.Fatal(err)
	}
	if !args.Options.IncludeFollowUps {
		t.Fatal("IncludeFollowUps = false, want true")
	}
	if args.NotebookID != "notebook" || args.ConversationID != "conversation" {
		t.Fatalf("arguments = %+v", args)
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
			parsed, err := tryParseChatCommandForTest(t, "chat show", test.args, globalOptions{})
			if test.wantErr {
				if err == nil {
					t.Fatal("error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			args, err := decodeChatShowArgs(parsed)
			if err != nil {
				t.Fatal(err)
			}
			if args.Options.OutFile != test.wantOut {
				t.Fatalf("OutFile = %q, want %q", args.Options.OutFile, test.wantOut)
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
			parsed, err := tryParseChatCommandForTest(t, "chat show", test.args, globalOptions{})
			if test.wantErr {
				if err == nil {
					t.Fatal("error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			args, err := decodeChatShowArgs(parsed)
			if err != nil {
				t.Fatal(err)
			}
			if args.Options.Format != test.wantFormat {
				t.Fatalf("format = %q, want %q", args.Options.Format, test.wantFormat)
			}
			if args.NotebookID != "notebook" || args.ConversationID != "" {
				t.Fatalf("arguments = %+v, want notebook only", args)
			}
		})
	}
}

func TestParseChatShowBackfill(t *testing.T) {
	parsed := parseChatCommandForTest(t, "chat show", []string{
		"--backfill", "notebook", "conversation",
	}, globalOptions{})
	args, err := decodeChatShowArgs(parsed)
	if err != nil {
		t.Fatal(err)
	}
	if !args.Options.Backfill {
		t.Fatal("Backfill = false, want true")
	}
	if args.NotebookID != "notebook" || args.ConversationID != "conversation" {
		t.Fatalf("arguments = %+v", args)
	}
}

func TestParseChatRenderOptionalFlags(t *testing.T) {
	tests := []struct {
		name           string
		flags          []string
		excerptBudget  int
		hideConfidence bool
		hideSpans      bool
	}{
		{
			name:          "bare excerpt",
			flags:         []string{"--citation-excerpts"},
			excerptBudget: defaultExcerptBudget,
		},
		{
			name:           "explicit values",
			flags:          []string{"--citation-excerpt=80", "--citation-confidence=off", "--citation-spans=off"},
			excerptBudget:  80,
			hideConfidence: true,
			hideSpans:      true,
		},
		{
			name:  "bare toggles show columns",
			flags: []string{"--citation-confidence=off", "--citation-confidence", "--citation-spans"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			values := append([]string{"nb"}, test.flags...)
			parsed := parseChatCommandForTest(t, "chat", values, globalOptions{})
			args, err := decodeChatArgs(parsed)
			if err != nil {
				t.Fatal(err)
			}
			render := args.Options.Render
			if render.ExcerptBudget != test.excerptBudget ||
				render.HideConfidence != test.hideConfidence ||
				render.HideSpans != test.hideSpans {
				t.Fatalf("render = %+v", render)
			}
		})
	}
}

func parseChatCommandForTest(
	t *testing.T,
	path string,
	values []string,
	globals globalOptions,
) parsedCommand {
	t.Helper()
	parsed, err := tryParseChatCommandForTest(t, path, values, globals)
	if err != nil {
		t.Fatalf("parseCommandSpec(%s): %v", path, err)
	}
	return parsed
}

func tryParseChatCommandForTest(
	t *testing.T,
	path string,
	values []string,
	globals globalOptions,
) (parsedCommand, error) {
	t.Helper()
	command, ok := lookupCommand(path)
	if !ok {
		t.Fatalf("%s command not found", path)
	}
	return parseCommandSpec(command.spec, command.surfaceSpec, values, globals)
}

func TestSaveChatSessionWritesConversationFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	session := &chatSession{
		NotebookID:     "nb",
		ConversationID: "12345678-1234-1234-1234-123456789abc",
		Messages:       []storedMessage{{Role: "user", Content: "hello"}},
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
	session := &chatSession{
		NotebookID:     "nb",
		ConversationID: "12345678-1234-1234-1234-123456789abc",
		Messages: []storedMessage{{
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
	active := &chatSession{
		NotebookID:     "nb",
		ConversationID: "aaaaaaaa-1234-1234-1234-123456789abc",
		Messages:       []storedMessage{{Role: "assistant", Content: "active"}},
	}
	if err := saveChatSession(active); err != nil {
		t.Fatal(err)
	}
	backfilled := &chatSession{
		NotebookID:     "nb",
		ConversationID: "bbbbbbbb-1234-1234-1234-123456789abc",
		Messages:       []storedMessage{{Role: "assistant", Content: "older"}},
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
		message           storedMessage
		rich              map[string]*pb.RichDocument
		citations         map[string][]api.Citation
		wantChanged       bool
		wantRich, wantCit int
	}{
		{
			name: "fills gaps",
			message: storedMessage{
				Role: "assistant", Content: "answer", Thinking: "keep me",
			},
			rich:        map[string]*pb.RichDocument{key: serverRich},
			citations:   map[string][]api.Citation{key: serverCitations},
			wantChanged: true, wantRich: 1, wantCit: 1,
		},
		{
			name: "preserves local data",
			message: storedMessage{
				Role: "assistant", Content: "answer", Thinking: "keep me",
				Rich: localRich, Citations: localCitations,
			},
			rich:      map[string]*pb.RichDocument{key: serverRich},
			citations: map[string][]api.Citation{key: serverCitations},
		},
		{
			name: "server flat leaves local flat",
			message: storedMessage{
				Role: "assistant", Content: "answer", Thinking: "keep me",
			},
			rich:      map[string]*pb.RichDocument{},
			citations: map[string][]api.Citation{},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			session := &chatSession{Messages: []storedMessage{test.message}}
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
