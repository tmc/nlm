package render

import (
	"io"

	"github.com/tmc/nlm/internal/richrender"
	"github.com/tmc/nlm/notebooklm"
)

// NoteText writes note as terminal-friendly text, flattening its rich
// document when present and appending resolved citations.
func NoteText(w io.Writer, note *notebooklm.Note) error {
	return richrender.RenderNoteText(w, richrender.NoteDocumentFromAPI(note))
}

// NoteMarkdown writes note as Markdown.
func NoteMarkdown(w io.Writer, note *notebooklm.Note) error {
	return richrender.RenderNoteMarkdown(w, richrender.NoteDocumentFromAPI(note))
}

// NoteHTML writes note as a self-contained HTML document.
func NoteHTML(w io.Writer, note *notebooklm.Note) error {
	return richrender.RenderNoteHTML(w, richrender.NoteDocumentFromAPI(note))
}
