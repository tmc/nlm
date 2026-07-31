package render

import (
	"github.com/tmc/nlm/internal/richrender"
	"github.com/tmc/nlm/notebooklm"
)

// SourceText returns a source's plain-text reading view.
//
// It reflows the byte-faithful [notebooklm.LoadSourceText.Full] output by
// collapsing the server's offset gaps into reading flow, so a region the
// NotebookLM index dropped reads as a paragraph break rather than a long run
// of spaces. A source with no gaps renders identically to Full.
func SourceText(body notebooklm.LoadSourceText) string {
	return richrender.SourceText(body)
}
