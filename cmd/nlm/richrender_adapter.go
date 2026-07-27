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

func collapseWhitespace(text string) string {
	return richrender.CollapseWhitespace(text)
}
