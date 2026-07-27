package richrender

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tmc/nlm/internal/designreview"
	"github.com/tmc/nlm/internal/notebooklm/api"
)

// ResolvedCitation holds what a per-citation source lookup produced. Today
// that is only a vim/quickfix-clickable "file:line:col" location, and only
// when the source is a txtar archive whose member the resolver could pin.
//
// NotebookLM's server-side indexer strips newlines from source bodies, which
// destroys the line structure txtar resolution relies on — so Location is
// often empty even for sources that were uploaded as txtar archives. The cited
// source text is not recovered here: the server ships it inline on the wire
// (api.Citation.Excerpt), so there is nothing to slice out of the body.
type ResolvedCitation struct {
	Location string // "file:line:col" for a resolved txtar member; "" otherwise
}

// resolveCitationLocations resolves chat citations to a txtar-aware
// "file:line:col" coordinate when possible. Returns nil if there are no
// citations or no loader. On per-source load failures the affected entries are
// simply missing from the result map (callers degrade to the unresolved label).
func resolveCitationLocations(load func(string) (api.LoadSourceText, error), cites []api.Citation) map[CitationKey]ResolvedCitation {
	if load == nil || len(cites) == 0 {
		return nil
	}
	bodies := make(map[string]api.LoadSourceText)
	out := make(map[CitationKey]ResolvedCitation)
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
		entry, ok := resolveOneCitation(body, c)
		if !ok {
			continue
		}
		out[keyFor(c)] = entry
	}
	return out
}

// resolveOneCitation resolves a single citation against an already-loaded
// source body to a txtar file:line location, when the resolver pinned a member
// file rather than falling back to the source title. It returns ok=false when
// no location could be produced. This is the single source of truth shared by
// the batch (resolveCitationLocations) and streaming-JSONL paths so the two
// never diverge.
func resolveOneCitation(body api.LoadSourceText, c api.Citation) (ResolvedCitation, bool) {
	if len(body.Fragments) == 0 {
		return ResolvedCitation{}, false
	}
	r := designreview.Resolve(body, designreview.NativeCitation{
		SourceID:   c.SourceID,
		StartChar:  c.StartChar,
		EndChar:    c.EndChar,
		Confidence: c.Confidence,
	})

	// A location is only meaningful when txtar resolution actually picked a
	// member file (not the raw source title) and produced a usable line number.
	if r.Status == designreview.StatusOK && r.File != "" && r.Line > 0 && r.File != body.Title {
		return ResolvedCitation{Location: formatLocation(r)}, true
	}
	return ResolvedCitation{}, false
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

// CitationKey identifies a Citation uniquely enough to look up its
// resolved location on a per-citation basis (multiple citations can
// share a SourceID but have distinct char ranges).
type CitationKey struct {
	SourceIndex int
	SourceID    string
	StartChar   int
	EndChar     int
}

func keyFor(c api.Citation) CitationKey {
	return CitationKey{SourceIndex: c.SourceIndex, SourceID: c.SourceID, StartChar: c.StartChar, EndChar: c.EndChar}
}
