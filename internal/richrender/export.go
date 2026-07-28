package richrender

import (
	"io"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/notebooklm/api"
)

const (
	// WeakConfidence is the threshold below which a citation is weakly grounded.
	WeakConfidence = weakConfidence
	// CitationModeOff suppresses trailing citation output.
	CitationModeOff = citationModeOff
	// CitationModeList emits a human-readable trailing citation list.
	CitationModeList = citationModeList
	// CitationModeJSON emits newline-delimited JSON events.
	CitationModeJSON = citationModeJSON
	// ANSIGrey starts the grey terminal style used for secondary output.
	ANSIGrey = ansiGrey
	// ANSIAmber starts the amber terminal style used for weak citations.
	ANSIAmber = ansiAmber
	// ANSIReset resets terminal styling.
	ANSIReset = ansiReset
)

// StreamOptions configures a StreamRenderer.
type StreamOptions struct {
	ShowThinking         bool
	Verbose              bool
	Mode                 CitationMode
	JSONL                bool
	JSONLIncludeThinking bool
	ResolveTitle         func(string) string
	SourceRemoved        func(string) bool
	LoadSource           func(string) (api.LoadSourceText, error)
	ExcerptBudget        int
	ShowConfidence       bool
	ShowSpans            bool
	Debug                io.Writer
}

// NewStreamRenderer returns a renderer that writes answer output and status separately.
func NewStreamRenderer(out, status io.Writer, opts StreamOptions) *StreamRenderer {
	renderer := newChatStreamRenderer(out, status, opts.ShowThinking, opts.Verbose, opts.Mode)
	renderer.jsonl = opts.JSONL
	renderer.jsonlIncludeThinking = opts.JSONLIncludeThinking
	renderer.resolveTitle = opts.ResolveTitle
	renderer.sourceRemoved = opts.SourceRemoved
	renderer.loadSource = opts.LoadSource
	renderer.excerptBudget = opts.ExcerptBudget
	renderer.showConfidence = opts.ShowConfidence
	renderer.showSpans = opts.ShowSpans
	renderer.debug = opts.Debug
	return renderer
}

// Citations returns the latest citation snapshot received by the renderer.
func (r *StreamRenderer) Citations() []api.Citation {
	return r.citations
}

// FollowUps returns the latest follow-up suggestions received by the renderer.
func (r *StreamRenderer) FollowUps() []string {
	return r.followUps
}

// ResolveCitationMode maps a command-line citation mode to its render mode.
func ResolveCitationMode(flag string) CitationMode {
	return resolveCitationMode(flag)
}

// WarnDeprecatedCitationMode reports a deprecated citation mode to w.
func WarnDeprecatedCitationMode(w io.Writer, flag string) {
	warnDeprecatedCitationMode(w, flag)
}

// CitationTitle returns the best available title for a citation.
func CitationTitle(citation api.Citation, resolveTitle func(string) string) string {
	return citationTitle(citation, resolveTitle)
}

// RichDocumentFromProto projects a protobuf rich document for rendering.
func RichDocumentFromProto(doc *pb.RichDocument) *RichDocument {
	return richDocumentFromProto(doc)
}

// NoteDocumentFromAPI projects an API note for rendering.
func NoteDocumentFromAPI(note *api.Note) NoteDocument {
	return noteDocumentFromAPI(note)
}

// RenderChatHTML writes a self-contained HTML conversation.
func RenderChatHTML(w io.Writer, doc ChatDocument, ctx RenderContext) error {
	return renderChatHTML(w, doc, ctx)
}

// RenderChatMarkdown writes a conversation as Markdown.
func RenderChatMarkdown(w io.Writer, doc ChatDocument, ctx RenderContext) error {
	return renderChatMarkdown(w, doc, ctx)
}

// RenderChatText writes a terminal-friendly conversation and citation status.
func RenderChatText(out, status io.Writer, doc ChatDocument, mode CitationMode, ctx RenderContext) error {
	return renderChatText(out, status, doc, mode, ctx)
}

// RenderNoteHTML writes a self-contained HTML note.
func RenderNoteHTML(w io.Writer, doc NoteDocument) error {
	return renderNoteHTML(w, doc)
}

// RenderNoteMarkdown writes a note as Markdown.
func RenderNoteMarkdown(w io.Writer, doc NoteDocument) error {
	return renderNoteMarkdown(w, doc)
}

// RenderNoteText writes a note as terminal-friendly text.
func RenderNoteText(w io.Writer, doc NoteDocument) error {
	return renderNoteText(w, doc)
}

// RenderNotebookHTML writes a self-contained HTML collection of conversations.
func RenderNotebookHTML(w io.Writer, docs []NotebookDocument, ctx RenderContext) error {
	return renderNotebookHTML(w, docs, ctx)
}

// CollapseWhitespace replaces whitespace runs with single spaces.
func CollapseWhitespace(text string) string {
	return collapseWhitespace(text)
}
