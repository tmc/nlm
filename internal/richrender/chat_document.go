package richrender

import (
	"encoding/json"
	"strings"

	"github.com/tmc/nlm/internal/notebooklm/api"
)

// shouldReflowFromTree reports whether an assistant answer should have its
// paragraph structure reconstructed from the span tree instead of rendered from
// flat Content. Reflow is a FLOOR, not the default: most answers ship Content
// with its newlines, literal [N] markers, and Markdown intact, and that flat
// text is the richer source (the tree's leaf text omits the [N] markers). So
// reflow applies only when there is a tree AND Content is a single run-together
// line with no structure of its own.
//
// Two exclusions keep it from mangling text that only looks unstructured:
//   - Content that already contains a newline is left alone (it has structure).
//   - Content that parses as JSON is left alone: a JSON-payload answer has no
//     literal newlines (they are escaped inside string values) yet reflowing it
//     by paragraph would corrupt the payload.
//
// Both renderers (text reflow, HTML block structure) gate on this so the
// decision lives in one place.
func shouldReflowFromTree(rich *RichDocument, content string) bool {
	if rich == nil {
		return false
	}
	if strings.ContainsRune(content, '\n') {
		return false
	}
	if looksLikeJSON(content) {
		return false
	}
	return true
}

// looksLikeJSON reports whether content is a JSON document (object or array).
// A newline-free answer that is really a JSON payload must not be reflowed by
// paragraph — its escaped internal newlines make it look unstructured when it is
// not. The cheap bracket check gates the full parse so the common prose answer
// pays nothing.
func looksLikeJSON(content string) bool {
	s := strings.TrimSpace(content)
	if len(s) < 2 {
		return false
	}
	if c := s[0]; c != '{' && c != '[' {
		return false
	}
	return json.Valid([]byte(s))
}

// ChatDocument is the format-neutral model of a rendered conversation. chatShow
// assembles one after it has swapped in excerpt-bearing history citations and
// resolved titles/locations; each output format (text, markdown, html) is a
// pure projection of it. This is the "one model, N renderers" seam: renderers
// read ChatDocument and never re-fetch or reach back into chatShow's loop.
type ChatDocument struct {
	NotebookID     string
	ConversationID string
	Title          string // human title of the conversation, "" if unknown
	Messages       []ChatMessage
}

// ChatMessage is one turn. Role is "user" or "assistant" (lower-case, as
// persisted). Citations are populated only for assistant turns and are already
// the best available copy (history excerpts swapped in when they were found).
type ChatMessage struct {
	Role      string
	Content   string
	Thinking  string // reasoning trace; only shown when the caller opted in
	Citations []api.Citation

	// Rich is the parsed answer-body span tree, populated only when the history
	// parse layer decodes one (see rich_document.go). It carries the document
	// structure the server strips from Content (all newlines are removed on the
	// wire), so renderers that see Rich != nil can reconstruct paragraphs, lists
	// and headings. It is strictly additive: every renderer keeps flat Content as
	// the floor and only consults Rich as a progressive enhancement.
	Rich *RichDocument
}

// RenderContext carries the per-render options and the optional resolution
// hooks a format renderer needs. resolveTitle maps a source ID to its notebook
// title; loadSource fetches a source body for txtar file:line resolution. Both
// may be nil (offline / unauthed), in which case renderers degrade to the data
// already on the citation.
type RenderContext struct {
	ShowThinking     bool
	ExcerptBudget    int  // >0 enables per-source excerpts, clipped to this many runes
	HideConfidence   bool // drop the p= column
	HideSpans        bool // drop the answer/src span labels
	IncludeFollowUps bool // retain generated trailing prompts in HTML

	ResolveTitle func(sourceID string) string
	LoadSource   func(sourceID string) (api.LoadSourceText, error)

	// sourceRemoved reports whether a citation's source ID is absent from the
	// notebook source list. A citation handle is a granular chunk/passage ID, so
	// it commonly misses the source list even when the underlying source is
	// present; the renderers therefore treat a miss as "title unavailable", not
	// "removed". It returns false when the list can't be determined (offline /
	// unauthed), so a renderer only shows the hint on solid evidence. May be nil.
	SourceRemoved func(sourceID string) bool
}

// citationSourceTitle returns the best display title for a citation: a resolved
// notebook title when available, else the server-supplied Title, else "". All
// three format renderers share this so titling never diverges between surfaces.
// It resolves off the parent source (ParentSourceID), since the chunk-level
// SourceID is not in the project source list; see citationSourceID.
func (ctx RenderContext) citationSourceTitle(c api.Citation) string {
	if ctx.ResolveTitle != nil {
		if t := ctx.ResolveTitle(citationSourceID(c)); t != "" {
			return t
		}
	}
	return c.Title
}

// citationSourceID returns the id to resolve a citation against the project
// source list: the parent source (ParentSourceID) when present, else the
// chunk-level SourceID for frames that carried no parent. A citation grounds a
// chunk of a source, so SourceID is a passage handle absent from the source
// list; the parent is the id that resolves to a title and to presence.
func citationSourceID(c api.Citation) string {
	if c.ParentSourceID != "" {
		return c.ParentSourceID
	}
	return c.SourceID
}

// citationSourceRemoved reports whether a citation's source is unresolved — its
// id is absent from the notebook source list — but only when it also has no
// title to show. A resolved title means the source rendered fine (present, or
// captured at save time), so the title-unavailable hint would be misleading; a
// titled citation is never flagged. Presence is checked against the parent
// source id, matching where the title resolves.
func (ctx RenderContext) citationSourceRemoved(c api.Citation) bool {
	if ctx.SourceRemoved == nil {
		return false
	}
	if ctx.citationSourceTitle(c) != "" {
		return false
	}
	return ctx.SourceRemoved(citationSourceID(c))
}

// citationLocations resolves the "file:line:col" locators for a set of
// citations via loadSource (the --resolve-citations txtar path), returning a
// map keyed by CitationKey. Returns nil when no loader is configured; callers
// that want the raw offset fall back to SourceStart/SourceEnd themselves. The
// resolve is batched so repeated citations into one source cost a single fetch.
func (ctx RenderContext) citationLocations(cites []api.Citation) map[CitationKey]string {
	if ctx.LoadSource == nil {
		return nil
	}
	resolved := resolveCitationLocations(ctx.LoadSource, cites)
	if len(resolved) == 0 {
		return nil
	}
	out := make(map[CitationKey]string, len(resolved))
	for k, rc := range resolved {
		out[k] = rc.Location
	}
	return out
}
