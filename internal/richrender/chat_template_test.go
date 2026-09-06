package richrender

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/tmc/nlm/notebooklm"
)

func TestMarkdownTemplate(t *testing.T) {
	doc := ChatDocument{NotebookID: "nb", ConversationID: "conversation", Messages: []ChatMessage{{Role: "assistant", Content: "**Unicode 🐢**\n\n```go\nfmt.Println(1)\n```", Thinking: "private", Citations: []notebooklm.Citation{{SourceIndex: 2, SourceID: "source", Title: "a|b", Excerpt: "line one\nline two", Confidence: .9, StartChar: 1, EndChar: 4}}}}}
	for _, test := range []struct {
		name, source, want string
		ctx                RenderContext
		citations          bool
	}{
		{name: "layout", source: `# {{.Vars.title}}{{range .Conversations}}{{range .Messages}}{{if eq .Role "assistant"}}: {{.Content}}{{end}}{{end}}{{end}}`, want: "# Report: " + doc.Messages[0].Content},
		{name: "hidden", source: `{{range .Conversations}}{{range .Messages}}{{.Thinking}}/{{len .Citations}}{{end}}{{end}}`, want: "/0"},
		{name: "metadata", source: `{{range .Conversations}}{{range .Messages}}{{.Thinking}}/{{range .Citations}}{{.Index}}/{{.Title}}/{{.Confidence}}/{{.AnswerSpan}}/{{.Excerpt}}{{end}}{{end}}{{end}}`, ctx: RenderContext{ShowThinking: true, ExcerptBudget: 100}, citations: true, want: "private/2/a|b/0.90/1–4/line one\nline two"},
		{name: "hidden metadata", source: `{{range .Conversations}}{{range .Messages}}{{range .Citations}}{{.Confidence}}/{{.AnswerSpan}}/{{.SourceSpan}}/{{.Location}}/{{.Excerpt}}{{end}}{{end}}{{end}}`, ctx: RenderContext{HideConfidence: true, HideSpans: true}, citations: true, want: "////"},
	} {
		t.Run(test.name, func(t *testing.T) {
			tmpl, err := ParseMarkdownTemplate("test", test.source)
			if err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			if err := tmpl.Render(&out, []ChatDocument{doc}, test.ctx, map[string]string{"title": "Report"}, test.citations); err != nil {
				t.Fatal(err)
			}
			if out.String() != test.want {
				t.Fatalf("got %q; want %q", out.String(), test.want)
			}
		})
	}
	tmpl, _ := ParseMarkdownTemplate("fragments", `{{range .Conversations}}{{.Markdown}}{{end}}`)
	var got, want bytes.Buffer
	if err := tmpl.Render(&got, []ChatDocument{doc}, RenderContext{}, nil, true); err != nil {
		t.Fatal(err)
	}
	if err := renderChatMarkdown(&want, doc, RenderContext{}); err != nil {
		t.Fatal(err)
	}
	if got.String() != want.String() {
		t.Fatal("fragment differs from default Markdown")
	}
	if doc.Messages[0].Thinking != "private" || len(doc.Messages[0].Citations) != 1 {
		t.Fatal("input mutated")
	}
}

func TestMarkdownTemplateErrors(t *testing.T) {
	if _, err := ParseMarkdownTemplate("broken", `{{`); err == nil {
		t.Fatal("accepted invalid syntax")
	}
	for _, source := range []string{`partial {{.Missing}}`, `partial {{.Vars.missing}}`} {
		tmpl, _ := ParseMarkdownTemplate("broken", source)
		var out bytes.Buffer
		if err := tmpl.Render(&out, nil, RenderContext{}, map[string]string{}, true); err == nil || !strings.Contains(err.Error(), "broken") {
			t.Fatalf("error = %v", err)
		}
		if out.Len() != 0 {
			t.Fatal("partial output")
		}
	}
	tmpl, _ := ParseMarkdownTemplate("writer", `hello`)
	if err := tmpl.Render(failedTemplateWriter{}, nil, RenderContext{}, nil, true); !errors.Is(err, errTemplateWriter) {
		t.Fatalf("writer error = %v", err)
	}
}

var errTemplateWriter = errors.New("writer failed")

type failedTemplateWriter struct{}

func (failedTemplateWriter) Write([]byte) (int, error) { return 0, errTemplateWriter }

func ExampleParseMarkdownTemplate() {
	layout, err := ParseMarkdownTemplate("report", `# {{.Vars.title}}
{{range .Conversations}}{{.Markdown}}{{end}}`)
	if err != nil {
		panic(err)
	}
	var out bytes.Buffer
	err = layout.Render(&out, []ChatDocument{{NotebookID: "nb", Messages: []ChatMessage{{Role: "assistant", Content: "A saved answer."}}}}, RenderContext{}, map[string]string{"title": "Report"}, true)
	if err != nil {
		panic(err)
	}
	fmt.Print(out.String())
	// Output:
	// # Report
	// #### ASSISTANT
	//
	// A saved answer.
}
