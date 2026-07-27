package richrender

import (
	"io"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/notebooklm/api"
)

type RichDocument = richDocument
type ChatDocument = chatDocument
type ChatMessage = chatDocMessage
type RenderContext = chatRenderContext
type NoteDocument = noteDocument
type NotebookDocument = notebookChatDocument
type CitationKey = citationKey
type ResolvedCitation = resolvedCitation
type CitationMode = citationRenderMode
type StreamRenderer = chatStreamRenderer

const (
	WeakConfidence   = weakConfidence
	CitationModeOff  = citationModeOff
	CitationModeList = citationModeList
	CitationModeJSON = citationModeJSON
	ANSIGrey         = ansiGrey
	ANSIAmber        = ansiAmber
	ANSIReset        = ansiReset
)

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

type StoredMessage struct {
	Role      string
	Content   string
	Thinking  string
	Citations []api.Citation
}

type PersistedOptions struct {
	ExcerptBudget  int
	HideConfidence bool
	HideSpans      bool
	LoadSource     func(string) (api.LoadSourceText, error)
	ResolveTitle   func(string) string
	SourceRemoved  func(string) bool
}

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

func (r *chatStreamRenderer) Citations() []api.Citation {
	return r.citations
}

func (r *chatStreamRenderer) FollowUps() []string {
	return r.followUps
}

func ResolveCitationMode(flag string) CitationMode {
	return resolveCitationMode(flag)
}

func WarnDeprecatedCitationMode(w io.Writer, flag string) {
	warnDeprecatedCitationMode(w, flag)
}

func CitationTitle(citation api.Citation, resolveTitle func(string) string) string {
	return citationTitle(citation, resolveTitle)
}

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

func RichDocumentFromProto(doc *pb.RichDocument) *RichDocument {
	return richDocumentFromProto(doc)
}

func NoteDocumentFromAPI(note *api.Note) NoteDocument {
	return noteDocumentFromAPI(note)
}

func RenderChatHTML(w io.Writer, doc ChatDocument, ctx RenderContext) error {
	return renderChatHTML(w, doc, ctx)
}

func RenderChatMarkdown(w io.Writer, doc ChatDocument, ctx RenderContext) error {
	return renderChatMarkdown(w, doc, ctx)
}

func RenderChatText(out, status io.Writer, doc ChatDocument, mode CitationMode, ctx RenderContext) error {
	return renderChatText(out, status, doc, mode, ctx)
}

func RenderNoteHTML(w io.Writer, doc NoteDocument) error {
	return renderNoteHTML(w, doc)
}

func RenderNoteMarkdown(w io.Writer, doc NoteDocument) error {
	return renderNoteMarkdown(w, doc)
}

func RenderNoteText(w io.Writer, doc NoteDocument) error {
	return renderNoteText(w, doc)
}

func RenderNotebookHTML(w io.Writer, docs []NotebookDocument, ctx RenderContext) error {
	return renderNotebookHTML(w, docs, ctx)
}

func ReflowAnswer(rich *RichDocument, content string) string {
	if !shouldReflowFromTree(rich, content) {
		return ""
	}
	return flattenText(projectRichDocument(rich))
}

func ResolveCitationLocations(load func(string) (api.LoadSourceText, error), citations []api.Citation) map[CitationKey]ResolvedCitation {
	return resolveCitationLocations(load, citations)
}

func ResolveOneCitation(body api.LoadSourceText, citation api.Citation) (ResolvedCitation, bool) {
	return resolveOneCitation(body, citation)
}

func KeyFor(citation api.Citation) CitationKey {
	return keyFor(citation)
}

func CitationSourceID(citation api.Citation) string {
	return citationSourceID(citation)
}

func GroupCitationsByIndex(citations []api.Citation) ([]int, map[int][]api.Citation) {
	return groupCitationsByIndex(citations)
}

func FormatAnswerSpan(start, end int) string {
	return formatAnswerSpan(start, end)
}

func FormatSourceSpan(start, end int) string {
	return formatSourceSpan(start, end)
}

func ShortSourceID(id string) string {
	return shortSourceID(id)
}

func TruncateExcerpt(text string, max int) string {
	return truncateExcerpt(text, max)
}

func ClipExcerpt(text string, max int) string {
	return clipExcerpt(text, max)
}

func DecodeNumberedExcerpt(text string) string {
	return decodeNumberedExcerpt(text)
}

func FormatFlattenedExcerptTable(text string) string {
	return formatFlattenedExcerptTable(text)
}

func ClipRunes(text string, max int) string {
	return clipRunes(text, max)
}

func CollapseWhitespace(text string) string {
	return collapseWhitespace(text)
}
