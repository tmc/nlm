package main

import (
	"fmt"

	"github.com/tmc/nlm/internal/designreview"
	"github.com/tmc/nlm/internal/notebooklm/api"
)

// resolveCitationLocations resolves chat citations to txtar-aware
// "file:line[-endLine]" coordinates. Returns nil if no citations or no
// loader. On per-source load failures the affected entries are simply
// missing from the result map (callers degrade to the unresolved label).
func resolveCitationLocations(load func(string) (api.LoadSourceText, error), cites []api.Citation) map[citationKey]string {
	if load == nil || len(cites) == 0 {
		return nil
	}
	bodies := make(map[string]api.LoadSourceText)
	out := make(map[citationKey]string)
	for _, c := range cites {
		if c.SourceID == "" {
			continue
		}
		body, ok := bodies[c.SourceID]
		if !ok {
			loaded, err := load(c.SourceID)
			if err != nil {
				bodies[c.SourceID] = api.LoadSourceText{} // negative cache
				continue
			}
			body = loaded
			bodies[c.SourceID] = body
		}
		if len(body.Fragments) == 0 {
			continue
		}
		r := designreview.Resolve(body, designreview.NativeCitation{
			SourceID:   c.SourceID,
			StartChar:  c.StartChar,
			EndChar:    c.EndChar,
			Confidence: c.Confidence,
		})
		// Only emit a location string when txtar resolution succeeded
		// (i.e. the resolver picked a member file rather than the raw
		// source title) and we got a usable line number.
		if r.Status != designreview.StatusOK || r.File == "" || r.Line <= 0 {
			continue
		}
		if r.File == body.Title {
			// Not a txtar member — single-file source, no benefit to
			// adding the line number alongside the existing label.
			continue
		}
		out[citationKey{SourceIndex: c.SourceIndex, SourceID: c.SourceID, StartChar: c.StartChar, EndChar: c.EndChar}] = formatLocation(r)
	}
	return out
}

// formatLocation renders a resolved citation as an editor-style path string:
//
//	file:line:col                          (citation fits on one line)
//	file:line:col-endLine:endCol           (multi-line span)
//	file:line                              (column missing — degraded)
//
// gopls, vim, gcc and most editors recognize the file:line:col form, so it
// pastes into a terminal and jumps straight to the right offset.
func formatLocation(r designreview.Resolved) string {
	if r.Line <= 0 {
		return r.File
	}
	if r.Column <= 0 {
		if r.EndLine > r.Line {
			return fmt.Sprintf("%s:%d-%d", r.File, r.Line, r.EndLine)
		}
		return fmt.Sprintf("%s:%d", r.File, r.Line)
	}
	if r.EndLine > r.Line {
		if r.EndColumn > 0 {
			return fmt.Sprintf("%s:%d:%d-%d:%d", r.File, r.Line, r.Column, r.EndLine, r.EndColumn)
		}
		return fmt.Sprintf("%s:%d:%d-%d", r.File, r.Line, r.Column, r.EndLine)
	}
	if r.EndColumn > r.Column {
		return fmt.Sprintf("%s:%d:%d-%d", r.File, r.Line, r.Column, r.EndColumn)
	}
	return fmt.Sprintf("%s:%d:%d", r.File, r.Line, r.Column)
}

// citationKey identifies a Citation uniquely enough to look up its
// resolved location on a per-citation basis (multiple citations can
// share a SourceID but have distinct char ranges).
type citationKey struct {
	SourceIndex int
	SourceID    string
	StartChar   int
	EndChar     int
}

func keyFor(c api.Citation) citationKey {
	return citationKey{SourceIndex: c.SourceIndex, SourceID: c.SourceID, StartChar: c.StartChar, EndChar: c.EndChar}
}
