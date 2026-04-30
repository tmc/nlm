package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tmc/nlm/internal/designreview"
	"github.com/tmc/nlm/internal/notebooklm/api"
)

// resolveCitationLocations resolves chat citations to txtar-aware
// "file:line:col" coordinates. Returns nil if no citations or no
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

// formatLocation renders a resolved citation as a vim/quickfix-clickable
// "file:line:col" string. The span's end isn't included — vim, gopls, gcc,
// and editor cmd-click handlers parse only the leading triple, and the
// snippet shown alongside already conveys what's cited.
//
// Absolute paths get shortened to a path relative to the current working
// directory when possible, so the output pastes cleanly under a repo root.
func formatLocation(r designreview.Resolved) string {
	file := shortenPath(r.File)
	if r.Line <= 0 {
		return file
	}
	if r.Column <= 0 {
		return fmt.Sprintf("%s:%d", file, r.Line)
	}
	return fmt.Sprintf("%s:%d:%d", file, r.Line, r.Column)
}

// shortenPath returns p relative to the current working directory when p is
// absolute and lives inside it (or in a sibling reachable via "..").
// Falls back to p unchanged on any error or when the relative form would be
// longer/uglier than the absolute one.
func shortenPath(p string) string {
	if p == "" || !filepath.IsAbs(p) {
		return p
	}
	cwd, err := os.Getwd()
	if err != nil {
		return p
	}
	rel, err := filepath.Rel(cwd, p)
	if err != nil {
		return p
	}
	// A relative path that climbs out of cwd more than once is harder to
	// click than the absolute form; keep absolute in that case.
	if strings.HasPrefix(rel, ".."+string(filepath.Separator)+"..") || rel == ".." {
		return p
	}
	if len(rel) >= len(p) {
		return p
	}
	return rel
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
