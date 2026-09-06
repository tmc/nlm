package richrender

import (
	"bytes"
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/template"
)

// MarkdownTemplate is a parsed layout for saved conversations.
// Construct one with ParseMarkdownTemplate.
type MarkdownTemplate struct{ template *template.Template }

// TemplateDocument is the data available to a Markdown template.
type TemplateDocument struct {
	NotebookID    string
	Vars          map[string]string
	Conversations []TemplateConversation
}

// TemplateConversation contains a conversation and reusable Markdown.
type TemplateConversation struct {
	ID, Title string
	Messages  []TemplateMessage
	Markdown  string
}

// TemplateMessage contains a turn with display options already applied.
type TemplateMessage struct {
	Role, Content, Thinking     string
	Citations                   []TemplateCitation
	CitationsMarkdown, Markdown string
}

// TemplateCitation is a display-ready citation. Hidden metadata is empty.
type TemplateCitation struct {
	Index                                        int
	SourceID, ParentSourceID, Title, Excerpt     string
	Confidence, AnswerSpan, SourceSpan, Location string
}

// ParseMarkdownTemplate parses a layout without accessing files or services.
func ParseMarkdownTemplate(name, source string) (*MarkdownTemplate, error) {
	t, err := template.New(name).Option("missingkey=error").Funcs(template.FuncMap{
		"upper": strings.ToUpper, "lower": strings.ToLower, "join": strings.Join, "quote": strconv.Quote,
	}).Parse(source)
	if err != nil {
		return nil, fmt.Errorf("parse template: %w", err)
	}
	return &MarkdownTemplate{template: t}, nil
}

// Render writes a complete document only after template execution succeeds.
// Resolution hooks are evaluated while preparing data, never by the template.
func (t *MarkdownTemplate) Render(w io.Writer, docs []ChatDocument, ctx RenderContext, vars map[string]string, citations bool) error {
	data := TemplateDocument{Vars: vars}
	for _, doc := range docs {
		data.NotebookID = doc.NotebookID
		conversation := TemplateConversation{ID: doc.ConversationID, Title: doc.Title}
		visible := doc
		visible.Messages = append([]ChatMessage(nil), doc.Messages...)
		for i, message := range visible.Messages {
			if !ctx.ShowThinking {
				message.Thinking = ""
			}
			if !citations {
				message.Citations = nil
			}
			visible.Messages[i] = message
			view := TemplateMessage{Role: message.Role, Content: message.Content, Thinking: message.Thinking}
			locations := ctx.citationLocations(message.Citations)
			for _, c := range message.Citations {
				citation := TemplateCitation{Index: c.SourceIndex, SourceID: c.SourceID, ParentSourceID: c.ParentSourceID, Title: ctx.citationSourceTitle(c)}
				if ctx.ExcerptBudget > 0 {
					citation.Excerpt = clipExcerpt(c.Excerpt, ctx.ExcerptBudget)
				}
				if !ctx.HideConfidence {
					citation.Confidence = scanConfidence(c.Confidence)
				}
				if !ctx.HideSpans {
					citation.AnswerSpan = spanRange(c.StartChar, c.EndChar)
					citation.SourceSpan = spanRange(c.SourceStart, c.SourceEnd)
					citation.Location = auditLocator(c, locations)
				}
				view.Citations = append(view.Citations, citation)
			}
			var fragment bytes.Buffer
			if len(message.Citations) > 0 {
				bw := &markdownWriter{w: &fragment}
				if ctx.ExcerptBudget > 0 {
					renderMarkdownAudit(bw, message.Citations, ctx)
				} else {
					renderMarkdownScan(bw, message.Citations, ctx)
				}
				if bw.err != nil {
					return bw.err
				}
			}
			view.CitationsMarkdown = fragment.String()
			fragment.Reset()
			if err := renderChatMarkdown(&fragment, ChatDocument{Messages: []ChatMessage{message}}, ctx); err != nil {
				return err
			}
			view.Markdown = fragment.String()
			conversation.Messages = append(conversation.Messages, view)
		}
		var fragment bytes.Buffer
		if err := renderChatMarkdown(&fragment, visible, ctx); err != nil {
			return err
		}
		conversation.Markdown = fragment.String()
		data.Conversations = append(data.Conversations, conversation)
	}
	var buf bytes.Buffer
	if err := t.template.Execute(&buf, data); err != nil {
		return fmt.Errorf("execute template: %w", err)
	}
	_, err := w.Write(buf.Bytes())
	return err
}
