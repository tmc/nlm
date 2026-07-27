package main

import (
	"io"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/notebooklm/api"
	"github.com/tmc/nlm/internal/richrender"
)

type richDocument = richrender.RichDocument
type chatDocument = richrender.ChatDocument
type chatDocMessage = richrender.ChatMessage
type chatRenderContext = richrender.RenderContext
type noteDocument = richrender.NoteDocument
type notebookChatDocument = richrender.NotebookDocument
type citationKey = richrender.CitationKey
type resolvedCitation = richrender.ResolvedCitation
type citationRenderMode = richrender.CitationMode
type chatStreamRenderer = richrender.StreamRenderer
type chatStreamOptions = richrender.StreamOptions

const (
	weakConfidence   = richrender.WeakConfidence
	citationModeOff  = richrender.CitationModeOff
	citationModeList = richrender.CitationModeList
	citationModeJSON = richrender.CitationModeJSON
	ansiGrey         = richrender.ANSIGrey
	ansiAmber        = richrender.ANSIAmber
	ansiReset        = richrender.ANSIReset
)

func newChatStreamRenderer(out, status io.Writer, opts chatStreamOptions) *chatStreamRenderer {
	return richrender.NewStreamRenderer(out, status, opts)
}

func resolveCitationMode(flag string) citationRenderMode {
	return richrender.ResolveCitationMode(flag)
}

func warnDeprecatedCitationMode(w io.Writer, flag string) {
	richrender.WarnDeprecatedCitationMode(w, flag)
}

func citationTitle(citation api.Citation, resolveTitle func(string) string) string {
	return richrender.CitationTitle(citation, resolveTitle)
}

func richDocumentFromProto(doc *pb.RichDocument) *richDocument {
	return richrender.RichDocumentFromProto(doc)
}

func noteDocumentFromAPI(note *api.Note) noteDocument {
	return richrender.NoteDocumentFromAPI(note)
}

func renderChatHTML(w io.Writer, doc chatDocument, ctx chatRenderContext) error {
	return richrender.RenderChatHTML(w, doc, ctx)
}

func renderChatMarkdown(w io.Writer, doc chatDocument, ctx chatRenderContext) error {
	return richrender.RenderChatMarkdown(w, doc, ctx)
}

func renderChatText(out, status io.Writer, doc chatDocument, mode citationRenderMode, ctx chatRenderContext) error {
	return richrender.RenderChatText(out, status, doc, mode, ctx)
}

func renderNoteHTML(w io.Writer, doc noteDocument) error {
	return richrender.RenderNoteHTML(w, doc)
}

func renderNoteMarkdown(w io.Writer, doc noteDocument) error {
	return richrender.RenderNoteMarkdown(w, doc)
}

func renderNoteText(w io.Writer, doc noteDocument) error {
	return richrender.RenderNoteText(w, doc)
}

func renderNotebookHTML(w io.Writer, docs []notebookChatDocument, ctx chatRenderContext) error {
	return richrender.RenderNotebookHTML(w, docs, ctx)
}

func reflowAnswer(rich *richDocument, content string) string {
	return richrender.ReflowAnswer(rich, content)
}

func resolveCitationLocations(load func(string) (api.LoadSourceText, error), citations []api.Citation) map[citationKey]resolvedCitation {
	return richrender.ResolveCitationLocations(load, citations)
}

func resolveOneCitation(body api.LoadSourceText, citation api.Citation) (resolvedCitation, bool) {
	return richrender.ResolveOneCitation(body, citation)
}

func keyFor(citation api.Citation) citationKey {
	return richrender.KeyFor(citation)
}

func citationSourceID(citation api.Citation) string {
	return richrender.CitationSourceID(citation)
}

func groupCitationsByIndex(citations []api.Citation) ([]int, map[int][]api.Citation) {
	return richrender.GroupCitationsByIndex(citations)
}

func formatAnswerSpan(start, end int) string {
	return richrender.FormatAnswerSpan(start, end)
}

func formatSourceSpan(start, end int) string {
	return richrender.FormatSourceSpan(start, end)
}

func shortSourceID(id string) string {
	return richrender.ShortSourceID(id)
}

func truncateExcerpt(text string, max int) string {
	return richrender.TruncateExcerpt(text, max)
}

func clipExcerpt(text string, max int) string {
	return richrender.ClipExcerpt(text, max)
}

func decodeNumberedExcerpt(text string) string {
	return richrender.DecodeNumberedExcerpt(text)
}

func formatFlattenedExcerptTable(text string) string {
	return richrender.FormatFlattenedExcerptTable(text)
}

func clipRunes(text string, max int) string {
	return richrender.ClipRunes(text, max)
}

func collapseWhitespace(text string) string {
	return richrender.CollapseWhitespace(text)
}

func renderPersistedAssistant(out, status io.Writer, m storedMessage, mode citationRenderMode, cfg persistedRenderConfig) {
	richrender.RenderPersistedAssistant(out, status, richrender.StoredMessage{
		Role:      m.Role,
		Content:   m.Content,
		Thinking:  m.Thinking,
		Citations: m.Citations,
	}, mode, richrender.PersistedOptions{
		ExcerptBudget:  cfg.excerptBudget,
		HideConfidence: cfg.hideConfidence,
		HideSpans:      cfg.hideSpans,
		LoadSource:     cfg.loadSource,
		ResolveTitle:   cfg.resolveTitle,
		SourceRemoved:  cfg.sourceRemoved,
	})
}
