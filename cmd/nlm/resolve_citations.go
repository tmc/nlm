package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/tmc/nlm/internal/designreview"
	"github.com/tmc/nlm/internal/notebooklm/api"
)

// defaultExcerptBudget is the char budget used when --citation-excerpt is
// passed bare (no =N). Sized to fit a couple of lines of prose without
// overwhelming the trailing citation block.
const defaultExcerptBudget = 160

// excerptBudgetFlag backs --citation-excerpt[=N]. As a bool-style flag it can
// be passed bare (Set("true") → default budget) or with an explicit char
// count (Set("80")). A zero or negative count disables excerpts. The zero
// value (flag absent) means excerpts are off.
type excerptBudgetFlag struct {
	set    bool
	budget int
}

// IsBoolFlag lets the flag be passed bare (--citation-excerpt) as well as with
// a value (--citation-excerpt=80). It is what the arg splitter keys off to
// decide whether the following token is this flag's value.
func (f *excerptBudgetFlag) IsBoolFlag() bool { return true }

func (f *excerptBudgetFlag) String() string {
	if f == nil || !f.set {
		return ""
	}
	return strconv.Itoa(f.budget)
}

func (f *excerptBudgetFlag) Set(v string) error {
	f.set = true
	switch v {
	case "", "true":
		f.budget = defaultExcerptBudget
		return nil
	case "false":
		f.budget = 0
		return nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fmt.Errorf("citation-excerpt: want a char count, got %q", v)
	}
	f.budget = n
	return nil
}

// Budget returns the effective excerpt char budget: 0 when the flag was not
// set or was set to a non-positive count (excerpts disabled).
func (f excerptBudgetFlag) Budget() int {
	if !f.set || f.budget <= 0 {
		return 0
	}
	return f.budget
}

// offToggleFlag backs a default-on column toggle like --citation-confidence and
// --citation-spans. The column shows by default; passing the flag bare, "=on",
// or "=true" keeps it on, while "=off"/"=false"/"=no" hides it. It is a
// bool-style flag so it can be written bare (which, being a no-op, is harmless)
// or with a value. Hidden reports whether the column should be suppressed.
type offToggleFlag struct {
	set    bool
	hidden bool
}

func (f *offToggleFlag) IsBoolFlag() bool { return true }

func (f *offToggleFlag) String() string {
	if f == nil || !f.set {
		return ""
	}
	if f.hidden {
		return "off"
	}
	return "on"
}

func (f *offToggleFlag) Set(v string) error {
	f.set = true
	switch strings.ToLower(v) {
	case "", "on", "true", "yes", "1":
		f.hidden = false
	case "off", "false", "no", "0":
		f.hidden = true
	default:
		return fmt.Errorf("want on/off, got %q", v)
	}
	return nil
}

// Hidden reports whether the toggle was set to suppress its column.
func (f offToggleFlag) Hidden() bool { return f.set && f.hidden }

// excerptBudgetSink is a --citation-excerpts adapter that writes the parsed
// budget directly into a target *int (the per-command chatRenderOptions field)
// rather than into an excerptBudgetFlag. It shares the same bare/=N parsing.
type excerptBudgetSink struct{ dst *int }

func (s *excerptBudgetSink) IsBoolFlag() bool { return true }

func (s *excerptBudgetSink) String() string {
	if s == nil || s.dst == nil || *s.dst <= 0 {
		return ""
	}
	return strconv.Itoa(*s.dst)
}

func (s *excerptBudgetSink) Set(v string) error {
	var f excerptBudgetFlag
	if err := f.Set(v); err != nil {
		return err
	}
	*s.dst = f.Budget()
	return nil
}

// offToggleSink is an offToggleFlag adapter that writes "hidden" into a target
// *bool (a per-command chatRenderOptions field). It shares the on/off parsing.
type offToggleSink struct{ hidden *bool }

func (s *offToggleSink) IsBoolFlag() bool { return true }

func (s *offToggleSink) String() string {
	if s == nil || s.hidden == nil || !*s.hidden {
		return ""
	}
	return "off"
}

func (s *offToggleSink) Set(v string) error {
	var f offToggleFlag
	if err := f.Set(v); err != nil {
		return err
	}
	*s.hidden = f.Hidden()
	return nil
}

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
func resolveCitationLocations(load func(string) (api.LoadSourceText, error), cites []api.Citation) map[citationKey]resolvedCitation {
	if load == nil || len(cites) == 0 {
		return nil
	}
	bodies := make(map[string]api.LoadSourceText)
	out := make(map[citationKey]resolvedCitation)
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
func resolveOneCitation(body api.LoadSourceText, c api.Citation) (resolvedCitation, bool) {
	if len(body.Fragments) == 0 {
		return resolvedCitation{}, false
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
		return resolvedCitation{Location: formatLocation(r)}, true
	}
	return resolvedCitation{}, false
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
