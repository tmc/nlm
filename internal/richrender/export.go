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
}

// StoredMessage is the persisted subset of an assistant message.
type StoredMessage struct {
	Role      string
	Content   string
	Thinking  string
	Citations []api.Citation
}

// PersistedOptions configures replay of a stored assistant message.
type PersistedOptions struct {
	ExcerptBudget  int
	HideConfidence bool
	HideSpans      bool
	LoadSource     func(string) (api.LoadSourceText, error)
	ResolveTitle   func(string) string
	SourceRemoved  func(string) bool
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

// RenderPersistedAssistant replays a stored assistant message through the stream renderer.
func RenderPersistedAssistant(out, status io.Writer, message StoredMessage, mode CitationMode, opts PersistedOptions) {
	renderPersistedAssistant(out, status, message, mode, persistedRenderConfig{
		excerptBudget:  opts.ExcerptBudget,
		hideConfidence: opts.HideConfidence,
		hideSpans:      opts.HideSpans,
		loadSource:     opts.LoadSource,
		resolveTitle:   opts.ResolveTitle,
		sourceRemoved:  opts.SourceRemoved,
	})
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

// ReflowAnswer reconstructs block boundaries when flat content has none.
func ReflowAnswer(rich *RichDocument, content string) string {
	if !shouldReflowFromTree(rich, content) {
		return ""
	}
	return flattenText(projectRichDocument(rich))
}

// ResolveCitationLocations resolves citation offsets against loaded source text.
func ResolveCitationLocations(load func(string) (api.LoadSourceText, error), citations []api.Citation) map[CitationKey]ResolvedCitation {
	return resolveCitationLocations(load, citations)
}

// ResolveOneCitation resolves one citation against a loaded source.
func ResolveOneCitation(body api.LoadSourceText, citation api.Citation) (ResolvedCitation, bool) {
	return resolveOneCitation(body, citation)
}

// KeyFor returns the stable key for a citation occurrence.
func KeyFor(citation api.Citation) CitationKey {
	return keyFor(citation)
}

// CitationSourceID returns the notebook source ID that owns a citation.
func CitationSourceID(citation api.Citation) string {
	return citationSourceID(citation)
}

// GroupCitationsByIndex groups citations by their visible source index.
func GroupCitationsByIndex(citations []api.Citation) ([]int, map[int][]api.Citation) {
	return groupCitationsByIndex(citations)
}

// FormatAnswerSpan formats an answer offset range.
func FormatAnswerSpan(start, end int) string {
	return formatAnswerSpan(start, end)
}

// FormatSourceSpan formats a source offset range.
func FormatSourceSpan(start, end int) string {
	return formatSourceSpan(start, end)
}

// ShortSourceID returns the display prefix of a source ID.
func ShortSourceID(id string) string {
	return shortSourceID(id)
}

// TruncateExcerpt truncates an excerpt and marks omitted content.
func TruncateExcerpt(text string, max int) string {
	return truncateExcerpt(text, max)
}

// ClipExcerpt clips an excerpt to at most max runes.
func ClipExcerpt(text string, max int) string {
	return clipExcerpt(text, max)
}

// DecodeNumberedExcerpt removes numbered-source escaping from an excerpt.
func DecodeNumberedExcerpt(text string) string {
	return decodeNumberedExcerpt(text)
}

// FormatFlattenedExcerptTable reconstructs a readable table from flattened text.
func FormatFlattenedExcerptTable(text string) string {
	return formatFlattenedExcerptTable(text)
}

// ClipRunes clips text to at most max runes.
func ClipRunes(text string, max int) string {
	return clipRunes(text, max)
}

// CollapseWhitespace replaces whitespace runs with single spaces.
func CollapseWhitespace(text string) string {
	return collapseWhitespace(text)
}
