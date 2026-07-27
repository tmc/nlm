package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/tmc/nlm/internal/notebooklm/api"
)

func TestRenderChatMarkdown(t *testing.T) {
	// A multi-source marker ([1] → three sources) plus a single-source marker
	// ([3]). Confidences differ per source; excerpts ride on the citations.
	multiSource := []api.Citation{
		{SourceIndex: 1, SourceID: "b149abcd-0000", Title: "codex: B149", StartChar: 115, EndChar: 241, Confidence: 0.91, Excerpt: "first excerpt text", SourceStart: 965670, SourceEnd: 966914},
		{SourceIndex: 1, SourceID: "7a59beef-1111", Title: "Skill triage", StartChar: 115, EndChar: 241, Confidence: 0.87, Excerpt: "ARCHITECTURE (user's suggestion, adopted): add native\nprofiling flags to the rank tools", SourceStart: 100, SourceEnd: 200},
		{SourceIndex: 1, SourceID: "e347cafe-2222", Title: "claude: E347", StartChar: 115, EndChar: 241, Confidence: 0.71},
		{SourceIndex: 3, SourceID: "b149abcd-0000", Title: "codex: B149", StartChar: 1244, EndChar: 1254, Confidence: 0.95, Excerpt: "second excerpt", SourceStart: 5, SourceEnd: 30},
	}

	tests := []struct {
		name string
		doc  chatDocument
		ctx  chatRenderContext
		want []string // substrings that must appear, in order
		deny []string // substrings that must NOT appear
	}{
		{
			name: "scan view table",
			doc: chatDocument{Messages: []chatDocMessage{
				{Role: "assistant", Content: "The answer.", Citations: multiSource},
			}},
			ctx: chatRenderContext{},
			want: []string{
				"#### ASSISTANT",
				"The answer.",
				"#### Citations",
				"| # | answer | p | source |",
				"|--:|:--|:--|:--|",
				"| 1 | 115–241 | 0.91 | b149abcd codex: B149 |",
				"|  |  | 0.87 | 7a59beef Skill triage |",
				"|  |  | 0.71 | e347cafe claude: E347 |",
				"| 3 | 1244–1254 | 0.95 | b149abcd codex: B149 |",
			},
			deny: []string{">", "p=0."},
		},
		{
			name: "scan view hides confidence",
			doc: chatDocument{Messages: []chatDocMessage{
				{Role: "assistant", Content: "A.", Citations: multiSource},
			}},
			ctx: chatRenderContext{HideConfidence: true},
			want: []string{
				"| # | answer | source |",
				"| 1 | 115–241 | b149abcd codex: B149 |",
			},
			deny: []string{"| p |", "0.91", "0.87"},
		},
		{
			name: "scan view hides spans",
			doc: chatDocument{Messages: []chatDocMessage{
				{Role: "assistant", Content: "A.", Citations: multiSource},
			}},
			ctx: chatRenderContext{HideSpans: true},
			want: []string{
				"| # | p | source |",
				"| 1 | 0.91 | b149abcd codex: B149 |",
			},
			deny: []string{"answer", "115–241"},
		},
		{
			name: "audit view blockquotes source-first",
			doc: chatDocument{Messages: []chatDocMessage{
				{Role: "assistant", Content: "A.", Citations: multiSource},
			}},
			ctx: chatRenderContext{ExcerptBudget: 200},
			want: []string{
				"#### Citations",
				// b149abcd grounds both [1] and [3] and appears once.
				"**b149abcd** — codex: B149 · src 965670–966914",
				"grounds [1] (p=0.91, answer 115–241) [3] (p=0.95, answer 1244–1254)",
				"> first excerpt text",
				"**7a59beef** — Skill triage · src 100–200",
				"grounds [1] (p=0.87, answer 115–241)",
				// The excerpt's internal newline is preserved: a multi-line cited
				// passage renders as a multi-line blockquote (each line prefixed
				// with "> "), not flattened to one space-joined line.
				"> ARCHITECTURE (user's suggestion, adopted): add native",
				"> profiling flags to the rank tools",
				"**e347cafe** — claude: E347",
				"grounds [1] (p=0.71, answer 115–241)",
			},
			deny: []string{"| # |"},
		},
		{
			name: "audit view hides confidence and spans",
			doc: chatDocument{Messages: []chatDocMessage{
				{Role: "assistant", Content: "A.", Citations: multiSource},
			}},
			ctx: chatRenderContext{ExcerptBudget: 200, HideConfidence: true, HideSpans: true},
			want: []string{
				"**b149abcd** — codex: B149",
				"grounds [1] [3]",
			},
			deny: []string{"p=0.", "answer 115", "· src"},
		},
		{
			name: "user and assistant two-turn",
			doc: chatDocument{Messages: []chatDocMessage{
				{Role: "user", Content: "What did codex say?"},
				{Role: "assistant", Content: "It said things.", Citations: multiSource[:1]},
			}},
			ctx: chatRenderContext{},
			want: []string{
				"#### USER",
				"What did codex say?",
				"#### ASSISTANT",
				"It said things.",
				"#### Citations",
				"| 1 | 115–241 | 0.91 | b149abcd codex: B149 |",
			},
		},
		{
			name: "assistant without citations omits section",
			doc: chatDocument{Messages: []chatDocMessage{
				{Role: "assistant", Content: "No sources here."},
			}},
			ctx:  chatRenderContext{},
			want: []string{"#### ASSISTANT", "No sources here."},
			deny: []string{"#### Citations", "|"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := renderChatMarkdown(&buf, tt.doc, tt.ctx); err != nil {
				t.Fatalf("renderChatMarkdown: %v", err)
			}
			got := buf.String()
			pos := 0
			for _, w := range tt.want {
				i := strings.Index(got[pos:], w)
				if i < 0 {
					t.Errorf("missing %q (in order) in output:\n%s", w, got)
					continue
				}
				pos += i + len(w)
			}
			for _, d := range tt.deny {
				if strings.Contains(got, d) {
					t.Errorf("unexpected %q in output:\n%s", d, got)
				}
			}
		})
	}
}

func TestRenderChatMarkdownEscapesTitles(t *testing.T) {
	doc := chatDocument{Messages: []chatDocMessage{
		{Role: "assistant", Content: "A.", Citations: []api.Citation{
			{SourceIndex: 1, SourceID: "aaaaaaaa-0000", Title: "a | b `c`", Confidence: 0.5},
		}},
	}}
	var buf bytes.Buffer
	if err := renderChatMarkdown(&buf, doc, chatRenderContext{}); err != nil {
		t.Fatalf("renderChatMarkdown: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, `aaaaaaaa a \| b \`+"`c\\`") {
		t.Errorf("title not escaped in output:\n%s", got)
	}
}

// TestRenderChatMarkdownPreservesExcerptNewlines pins F1: a cited passage with
// internal structure (newlines, indentation) — code, config, a table — must
// keep that structure in the blockquote, not be flattened to one line. Each
// line is prefixed with "> " so it stays a single quote block, and a blank
// interior line becomes a bare ">".
func TestRenderChatMarkdownPreservesExcerptNewlines(t *testing.T) {
	excerpt := "func main() {\n\tfmt.Println(\"hi\")\n\n\treturn\n}"
	doc := chatDocument{Messages: []chatDocMessage{
		{Role: "assistant", Content: "See the entrypoint [1].", Citations: []api.Citation{
			{SourceIndex: 1, SourceID: "codeabcd-1", Title: "main.go", Excerpt: excerpt, StartChar: 4, EndChar: 18, Confidence: 0.9},
		}},
	}}
	var buf bytes.Buffer
	if err := renderChatMarkdown(&buf, doc, chatRenderContext{ExcerptBudget: 200}); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	for _, want := range []string{
		"> func main() {",
		"> \tfmt.Println(\"hi\")",
		">", // the blank interior line
		"> \treturn",
		"> }",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("blockquote line %q missing; excerpt was flattened:\n%s", want, got)
		}
	}
	// The old flattened form must NOT appear.
	if strings.Contains(got, "func main() { \tfmt.Println") {
		t.Errorf("excerpt was space-joined instead of line-preserved:\n%s", got)
	}
}

// TestTruncateVsClipExcerpt pins the split behind F1: truncateExcerpt (TUI,
// single-line rows) collapses whitespace; clipExcerpt (HTML/Markdown, structured
// surfaces) preserves it. Both clip to the rune budget.
func TestTruncateVsClipExcerpt(t *testing.T) {
	in := "a\n\tb   c"
	if got, want := truncateExcerpt(in, 100), "a b c"; got != want {
		t.Errorf("truncateExcerpt should collapse whitespace: got %q want %q", got, want)
	}
	if got, want := clipExcerpt(in, 100), "a\n\tb   c"; got != want {
		t.Errorf("clipExcerpt should preserve internal whitespace: got %q want %q", got, want)
	}
	// clipExcerpt trims only the ends, not the interior.
	if got, want := clipExcerpt("  x\ny  ", 100), "x\ny"; got != want {
		t.Errorf("clipExcerpt should trim ends only: got %q want %q", got, want)
	}
	// Both clip to the budget with an ellipsis.
	if got := clipExcerpt("abcdef", 3); got != "abc…" {
		t.Errorf("clipExcerpt budget: got %q", got)
	}
}
