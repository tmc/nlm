package richrender

import (
	"strings"
	"testing"

	"github.com/tmc/nlm/internal/notebooklm/api"
)

func TestChatMarkdownSubset(t *testing.T) {
	doc := ChatDocument{Messages: []ChatMessage{{
		Role: "assistant",
		Content: "### Heading\n\n**Bold** and *italic* with `code` [1,2].\n\n" +
			"- first\n  - nested\n\n1. one\n2. two\n\n---\n\n$k$",
		Citations: []api.Citation{
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
