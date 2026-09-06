package richrender

import (
	"strings"
	"testing"

	"github.com/tmc/nlm/notebooklm"
)

func TestChatMarkdownSubset(t *testing.T) {
	doc := ChatDocument{Messages: []ChatMessage{{
		Role: "assistant",
		Content: "### Heading\n\n**Bold** and *italic* with `code` [1,2].\n\n" +
			"- first\n  - nested\n\n1. one\n2. two\n\n---\n\n$k$",
		Citations: []notebooklm.Citation{
			{SourceIndex: 1, SourceID: "source-1"},
			{SourceIndex: 2, SourceID: "source-2"},
		},
	}}}
	html := renderToString(t, doc, RenderContext{})
	for _, want := range []string{
		"<h3>Heading</h3>",
		"<strong>Bold</strong>",
		"<em>italic</em>",
		"<code>code</code>",
		"<ul>",
		`class="nest-1"`,
		"<ol>",
		"<hr>",
		`data-cite="1"`,
		`data-cite="2"`,
		"$k$",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("HTML does not contain %q", want)
		}
	}
	for _, raw := range []string{"### Heading", "**Bold**"} {
		if strings.Contains(answerBodies(html), raw) {
			t.Errorf("visible answer contains raw Markdown %q", raw)
		}
	}
}

func TestChatMarkdownSubsetConservative(t *testing.T) {
	const content = `{"heading":"### literal","items":["* literal"]}`
	doc := ChatDocument{Messages: []ChatMessage{{Role: "assistant", Content: content}}}
	html := renderToString(t, doc, RenderContext{})
	body := answerBodies(html)
	if !strings.Contains(body, `class="answer-block"`) || strings.Contains(body, "<h3>") {
		t.Fatalf("JSON answer was structured: %s", answerBodies(html))
	}
}

func TestChatFollowUps(t *testing.T) {
	tests := []struct {
		name   string
		prompt string
	}{
		{
			name:   "question",
			prompt: "📊 Would you like to generate a publication-quality chart?",
		},
		{
			name:   "bold next step",
			prompt: "📊 **Next Step**: Would you like to run a simulation?",
		},
		{
			name:   "plain next steps",
			prompt: "Next Steps: Would you like to run a simulation?",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			content := "Useful answer.\n\n---\n\n" + test.prompt
			doc := ChatDocument{Messages: []ChatMessage{{
				Role:    "assistant",
				Content: content,
			}}}
			html := renderToString(t, doc, RenderContext{})
			if strings.Contains(answerBodies(html), "Would you like") {
				t.Fatal("default HTML includes follow-up prompt")
			}
			html = renderToString(t, doc, RenderContext{IncludeFollowUps: true})
			if !strings.Contains(answerBodies(html), "Would you like") {
				t.Fatal("--include-follow-ups HTML omits follow-up prompt")
			}
			if doc.Messages[0].Content != content {
				t.Fatal("renderer changed the document")
			}
		})
	}
}

func TestChatFollowUpsConservative(t *testing.T) {
	tests := []string{
		"Useful answer. Would you like to continue?",
		"Useful answer.\n\nThe next step matters. Would you like to continue?",
		"Useful answer.\n\nNext Step: Run the simulation.",
	}
	for _, content := range tests {
		doc := ChatDocument{Messages: []ChatMessage{{
			Role:    "assistant",
			Content: content,
		}}}
		html := renderToString(t, doc, RenderContext{})
		if !strings.Contains(answerBodies(html), content) {
			t.Errorf("default HTML suppressed ordinary prose %q", content)
		}
	}
}

func answerBodies(html string) string {
	const start = `<template class="answer-body"`
	var out strings.Builder
	for {
		i := strings.Index(html, start)
		if i < 0 {
			return out.String()
		}
		html = html[i+len(start):]
		j := strings.Index(html, "</template>")
		if j < 0 {
			return out.String()
		}
		out.WriteString(html[:j])
		html = html[j+len("</template>"):]
	}
}

func TestMarkdownWithRichMetadata(t *testing.T) {
	content := "### Findings\n\n**Evidence** [1].\n\n- first\n\n> a quotation\n\n---\n🕵️ Would you like to trace the exact DNS logs?"
	doc := ChatDocument{Messages: []ChatMessage{{Role: "assistant", Content: content, Rich: &RichDocument{}, Citations: []notebooklm.Citation{{SourceIndex: 1, SourceID: "one"}}}}}
	body := answerBodies(renderToString(t, doc, RenderContext{}))
	for _, want := range []string{"<h3>Findings</h3>", "<strong>Evidence</strong>", "<ul>", "<blockquote>", `data-cite="1"`} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q: %s", want, body)
		}
	}
	if strings.Contains(body, "Would you like") {
		t.Fatal("follow-up retained")
	}
	if !strings.Contains(answerBodies(renderToString(t, doc, RenderContext{IncludeFollowUps: true})), "Would you like") {
		t.Fatal("explicit follow-up omitted")
	}
	if doc.Messages[0].Content != content {
		t.Fatal("input mutated")
	}
}

func TestFollowUpInsideCodePreserved(t *testing.T) {
	content := "Example:\n\n```text\n\nWould you like to continue?"
	doc := ChatDocument{Messages: []ChatMessage{{Role: "assistant", Content: content}}}
	if got := withoutChatFollowUps(doc).Messages[0].Content; got != content {
		t.Fatalf("got %q", got)
	}
}

func TestMarkdownExternalLinks(t *testing.T) {
	content := "[Discord message](https://discord.com/channels/1/2/3) and <https://example.test/a>.\n\nhttps://example.test/b.\n\n`https://example.test/code`\n\n[javascript](javascript:alert(1))"
	body := answerBodies(renderToString(t, ChatDocument{Messages: []ChatMessage{{Role: "assistant", Content: content}}}, RenderContext{}))
	for _, want := range []string{`href="https://discord.com/channels/1/2/3"`, `href="https://example.test/a"`, `href="https://example.test/b"`, `<code>https://example.test/code</code>`} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %s in %s", want, body)
		}
	}
	for _, bad := range []string{`href="javascript:`, `href="https://example.test/code"`, `href="https://example.test/b."`} {
		if strings.Contains(body, bad) {
			t.Errorf("unexpected %s", bad)
		}
	}
}

func TestMarkdownOrderedListStart(t *testing.T) {
	content := "1. First\n\n   - Detail\n\n2. Second\n\n   - Detail\n\n3. Third"
	body := answerBodies(renderToString(t, ChatDocument{Messages: []ChatMessage{{Role: "assistant", Content: content}}}, RenderContext{}))
	for _, want := range []string{"<ol>", `<ol start="2">`, `<ol start="3">`} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q in %s", want, body)
		}
	}
}

func TestMarkdownWrappedListItem(t *testing.T) {
	content := "1. First line\n   continued [1]\n2. Second item"
	doc := ChatDocument{Messages: []ChatMessage{{
		Role: "assistant", Content: content,
		Citations: []notebooklm.Citation{{SourceIndex: 1, SourceID: "one"}},
	}}}
	body := answerBodies(renderToString(t, doc, RenderContext{}))
	if strings.Count(body, "<ol>") != 1 || strings.Count(body, "<li>") != 2 || !strings.Contains(body, "First line\n   continued ") {
		t.Fatalf("wrapped item split: %s", body)
	}
	if !strings.Contains(body, `href="#cite-0-1"`) {
		t.Fatal("wrapped item lost citation")
	}
}

func TestMarkdownCodeInsideEmphasis(t *testing.T) {
	content := "**The `example.go` file** and *the `value` field*"
	body := answerBodies(renderToString(t, ChatDocument{Messages: []ChatMessage{{Role: "assistant", Content: content}}}, RenderContext{}))
	for _, want := range []string{"<strong>The <code>example.go</code> file</strong>", "<em>the <code>value</code> field</em>"} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %s in %s", want, body)
		}
	}
}
