package richrender

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/tmc/nlm/internal/designreview"
	"github.com/tmc/nlm/internal/notebooklm/api"
)

// resolvedCitation holds what a per-citation source lookup produced. Today
// that is only a vim/quickfix-clickable "file:line:col" location, and only
// when the source is a txtar archive whose member the resolver could pin.
//
// NotebookLM's server-side indexer strips newlines from source bodies, which
// destroys the line structure txtar resolution relies on — so Location is
// often empty even for sources that were uploaded as txtar archives. The cited
// source text is not recovered here: the server ships it inline on the wire
// (api.Citation.Excerpt), so there is nothing to slice out of the body.
type resolvedCitation struct {
	Location string // "file:line:col" for a resolved txtar member; "" otherwise
}

// resolveCitationLocations resolves chat citations to a txtar-aware
// "file:line:col" coordinate when possible. Returns nil if there are no
// citations or no loader. On per-source load failures the affected entries are
// simply missing from the result map (callers degrade to the unresolved label).
func resolveCitationLocations(load func(string) (api.LoadSourceText, error), cites []api.Citation, debug io.Writer) map[citationKey]resolvedCitation {
	if load == nil || len(cites) == 0 {
		return nil
	}
	bodies := make(map[string]api.LoadSourceText)
	loadFailed := make(map[string]bool)
	out := make(map[citationKey]resolvedCitation)
	seen := make(map[citationKey]bool)
	for _, c := range cites {
		key := keyFor(c)
		if seen[key] {
			continue
		}
		seen[key] = true
		sourceID := citationSourceID(c)
		if sourceID == "" {
			writeCitationDiagnostic(debug, c, "title-only")
			continue
		}
		body, ok := bodies[sourceID]
		if !ok {
			loaded, err := load(sourceID)
			if err != nil {
				bodies[sourceID] = api.LoadSourceText{} // negative cache
				loadFailed[sourceID] = true
				writeCitationDiagnostic(debug, c, "load error")
				continue
			}
			body = loaded
			bodies[sourceID] = body
		}
		if loadFailed[sourceID] {
			writeCitationDiagnostic(debug, c, "load error")
			continue
		}
		entry, ok, reason := resolveOneCitation(body, c)
		if !ok {
			writeCitationDiagnostic(debug, c, reason)
			continue
		}
		out[key] = entry
	}
	return out
}

// resolveOneCitation resolves a single citation against an already-loaded
// source body to a txtar file:line location, when the resolver pinned a member
// file rather than falling back to the source title. It returns ok=false when
// no location could be produced. This is the single source of truth shared by
// the batch (resolveCitationLocations) and streaming-JSONL paths so the two
// never diverge.
func resolveOneCitation(body api.LoadSourceText, c api.Citation) (resolvedCitation, bool, string) {
	if len(body.Fragments) == 0 {
		return resolvedCitation{}, false, "no members"
	}
	start, end := citationSourceRange(c)
	r := designreview.ResolveCitation(body, designreview.NativeCitation{
		SourceID:   citationSourceID(c),
		StartChar:  start,
		EndChar:    end,
		Confidence: c.Confidence,
	}, c.Excerpt)

	// A location is only meaningful when txtar resolution actually picked a
	// member file (not the raw source title), produced a usable line number,
	// and reproduced the server-provided excerpt.
	if r.Status == designreview.StatusHeaderSpan &&
		len(r.Members) > 1 &&
		designreview.ExcerptMatches(r.Snippet, c.Excerpt) {
		return resolvedCitation{Location: formatLocation(r)}, true, ""
	}
	if r.Status == designreview.StatusOK &&
		r.File != "" &&
		r.Line > 0 &&
		r.File != body.Title &&
		designreview.ExcerptMatches(r.Snippet, c.Excerpt) {
		return resolvedCitation{Location: formatLocation(r)}, true, ""
	}
	switch {
	case r.Status == designreview.StatusOffsetMiss && strings.Contains(r.Reason, "excerpt"):
		return resolvedCitation{}, false, "excerpt mismatch"
	case r.Status == designreview.StatusOffsetMiss:
		return resolvedCitation{}, false, "offset miss"
	case !designreview.ExcerptMatches(r.Snippet, c.Excerpt):
		return resolvedCitation{}, false, "excerpt mismatch"
	case r.Status == designreview.StatusHeaderSpan:
		return resolvedCitation{}, false, "header span"
	case r.File == body.Title:
		return resolvedCitation{}, false, "no members"
	default:
		return resolvedCitation{}, false, "title-only"
	}
}

func writeCitationDiagnostic(w io.Writer, c api.Citation, reason string) {
	if w == nil || reason == "" {
		return
	}
	fmt.Fprintf(w, "nlm: citation %d: %s\n", c.SourceIndex, reason)
}

// citationSourceRange returns the citation's span in the source document.
// Older captures did not carry source offsets, so fall back to the answer
// offsets for compatibility with those saved sessions.
func citationSourceRange(c api.Citation) (start, end int) {
	if c.SourceStart < c.SourceEnd {
		return c.SourceStart, c.SourceEnd
	}
	return c.StartChar, c.EndChar
}

// formatLocation renders a resolved citation as a vim/quickfix-clickable
// "file:line:col" string. The span's end isn't included — vim, gopls, gcc,
// and editor cmd-click handlers parse only the leading triple, and the
// snippet shown alongside already conveys what's cited.
//
// Absolute paths get shortened to a path relative to the current working
// directory when possible, so the output pastes cleanly under a repo root.
func formatLocation(r designreview.Resolved) string {
	if len(r.Members) > 1 {
		first := shortenPath(r.Members[0])
		last := shortenPath(r.Members[len(r.Members)-1])
		return fmt.Sprintf("%s … %s (%d files)", first, last, len(r.Members))
	}
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
