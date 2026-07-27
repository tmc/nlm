package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/tmc/nlm/internal/notebooklm/api"
)

// A citation whose source cannot be resolved to a titled notebook source: no
// title on the citation, and the notebook source list does not contain its ID
// (a citation handle is a granular chunk ID, so it legitimately misses the
// source list). present is the one source ID the (stubbed) notebook knows about.
const (
	removedSourceID = "6b1760c7-8e7c-4d97-b905-f284897b80c6"
	presentSourceID = "11111111-2222-3333-4444-555555555555"
)

// removedCtx builds a render context whose sourceRemoved hook reports every ID
// except presentSourceID as absent from the source list, mirroring a citation
// whose source could not be resolved. resolveTitle knows only the present
// source, so the unresolved one has no title from any source.
func removedCtx(budget int) chatRenderContext {
	return chatRenderContext{
		ExcerptBudget: budget,
		resolveTitle: func(id string) string {
			if id == presentSourceID {
				return "auth.go"
			}
			return ""
		},
		sourceRemoved: func(id string) bool { return id != presentSourceID },
	}
}

func TestCitationSourceRemovedGating(t *testing.T) {
	ctx := removedCtx(0)

	// Removed + untitled → flagged.
	if !ctx.citationSourceRemoved(api.Citation{SourceID: removedSourceID}) {
		t.Errorf("untitled removed source should be flagged removed")
	}
	// A server-supplied title means the source is not treated as removed, even
	// when the hook would say the current notebook lacks the ID — a titled
	// citation is never "removed".
	if ctx.citationSourceRemoved(api.Citation{SourceID: removedSourceID, Title: "was-titled"}) {
		t.Errorf("titled citation must not be flagged removed")
	}
	// A present, resolvable source is not removed.
	if ctx.citationSourceRemoved(api.Citation{SourceID: presentSourceID}) {
		t.Errorf("present source must not be flagged removed")
	}
	// No hook → never removed (offline replay must not claim removal).
	if (chatRenderContext{}).citationSourceRemoved(api.Citation{SourceID: removedSourceID}) {
		t.Errorf("without a sourceRemoved hook nothing is removed")
	}
}

func TestCitationSourceRemovedHTML(t *testing.T) {
	doc := chatDocument{Messages: []chatDocMessage{{
		Role:    "assistant",
		Content: "Grounded claim. [2]",
		Citations: []api.Citation{
			{SourceIndex: 2, SourceID: removedSourceID, Confidence: 0.95, Excerpt: "-- NOTES.md -- cited text"},
			{SourceIndex: 2, SourceID: presentSourceID, Confidence: 0.90, Excerpt: "present excerpt"},
		},
	}}}

	payload := decodeHTMLPayload(t, renderToString(t, doc, removedCtx(200)))
	if len(payload.Messages) != 1 || len(payload.Messages[0].Markers) != 1 {
		t.Fatalf("expected one marker, got %+v", payload.Messages)
	}
	sources := payload.Messages[0].Markers[0].Sources
	if len(sources) != 2 {
		t.Fatalf("expected 2 sources, got %d", len(sources))
	}
	for _, s := range sources {
		switch s.SourceID {
		case removedSourceID:
			if !s.Removed {
				t.Errorf("removed source %s: Removed=false, want true", s.Handle)
			}
			if s.Title != "" {
				t.Errorf("removed source should have no title, got %q", s.Title)
			}
		case presentSourceID:
			if s.Removed {
				t.Errorf("present source %s flagged Removed", s.Handle)
			}
			if s.Title != "auth.go" {
				t.Errorf("present source title = %q, want auth.go", s.Title)
			}
		}
	}
}

func TestCitationSourceRemovedText(t *testing.T) {
	doc := chatDocument{Messages: []chatDocMessage{{
		Role:    "assistant",
		Content: "Grounded claim. [2]",
		Citations: []api.Citation{
			{SourceIndex: 2, SourceID: removedSourceID, Confidence: 0.95},
			{SourceIndex: 2, SourceID: presentSourceID, Confidence: 0.90},
		},
	}}}

	var out, status bytes.Buffer
	if err := renderChatText(&out, &status, doc, citationModeList, removedCtx(0)); err != nil {
		t.Fatalf("renderChatText: %v", err)
	}
	got := status.String() // citation list prints to status
	if !strings.Contains(got, "6b1760c7 (title unavailable)") {
		t.Errorf("unresolved source row missing hint; got:\n%s", got)
	}
	if !strings.Contains(got, `11111111 "auth.go"`) {
		t.Errorf("present source row should show title; got:\n%s", got)
	}
}

func TestCitationSourceRemovedMarkdownScan(t *testing.T) {
	doc := chatDocument{Messages: []chatDocMessage{{
		Role:    "assistant",
		Content: "Grounded claim.",
		Citations: []api.Citation{
			{SourceIndex: 1, SourceID: removedSourceID, StartChar: 5, EndChar: 20, Confidence: 0.95},
			{SourceIndex: 1, SourceID: presentSourceID, StartChar: 5, EndChar: 20, Confidence: 0.90},
		},
	}}}

	var buf bytes.Buffer
	if err := renderChatMarkdown(&buf, doc, removedCtx(0)); err != nil {
		t.Fatalf("renderChatMarkdown: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "6b1760c7 (untitled)") {
		t.Errorf("unresolved source cell missing hint; got:\n%s", got)
	}
	if !strings.Contains(got, "11111111 auth.go") {
		t.Errorf("present source cell should show title; got:\n%s", got)
	}
}

func TestCitationSourceRemovedMarkdownAudit(t *testing.T) {
	doc := chatDocument{Messages: []chatDocMessage{{
		Role:    "assistant",
		Content: "Grounded claim.",
		Citations: []api.Citation{
			{SourceIndex: 1, SourceID: removedSourceID, StartChar: 5, EndChar: 20, Confidence: 0.95},
		},
	}}}

	var buf bytes.Buffer
	if err := renderChatMarkdown(&buf, doc, removedCtx(200)); err != nil {
		t.Fatalf("renderChatMarkdown: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "**6b1760c7** — _(title unavailable)_") {
		t.Errorf("unresolved source audit header missing hint; got:\n%s", got)
	}
}

// A source with no title but present in the notebook source list must NOT get
// the title-unavailable hint on any surface: the hint is reserved for sources
// the list did not resolve. sourceRemoved returns false for present IDs, so a
// bare handle is the correct rendering here — not a hint.
func TestCitationPresentUntitledNotRemoved(t *testing.T) {
	ctx := chatRenderContext{
		resolveTitle:  func(string) string { return "" }, // present but untitled
		sourceRemoved: func(string) bool { return false },
	}
	doc := chatDocument{Messages: []chatDocMessage{{
		Role:      "assistant",
		Content:   "Claim.",
		Citations: []api.Citation{{SourceIndex: 1, SourceID: presentSourceID, Confidence: 0.9}},
	}}}

	var buf bytes.Buffer
	if err := renderChatMarkdown(&buf, doc, ctx); err != nil {
		t.Fatalf("renderChatMarkdown: %v", err)
	}
	if got := buf.String(); strings.Contains(got, "untitled") || strings.Contains(got, "unavailable") {
		t.Errorf("present untitled source must not read as unresolved; got:\n%s", got)
	}
}

// The guardrail the wire evidence demands: when no sourceRemoved hook is wired
// (offline replay, or a source-list fetch that failed), renderers must NOT
// print the title-unavailable hint. removed() already returns false when the
// list is unavailable; this pins that the RENDER layer stays silent too, so a
// good source is never mislabeled on incomplete information.
func TestCitationNoRemovedHookNoHint(t *testing.T) {
	ctx := chatRenderContext{
		resolveTitle: func(string) string { return "" }, // no title resolvable
		// sourceRemoved deliberately nil: list unavailable / offline replay.
	}
	doc := chatDocument{Messages: []chatDocMessage{{
		Role:    "assistant",
		Content: "Grounded claim. [1]",
		Citations: []api.Citation{
			{SourceIndex: 1, SourceID: removedSourceID, StartChar: 8, EndChar: 14, Confidence: 0.95, Excerpt: "-- NOTES.md -- x"},
		},
	}}}

	var md bytes.Buffer
	if err := renderChatMarkdown(&md, doc, ctx); err != nil {
		t.Fatalf("renderChatMarkdown: %v", err)
	}
	if got := md.String(); strings.Contains(got, "untitled") || strings.Contains(got, "unavailable") {
		t.Errorf("no hook: markdown must not print a title hint; got:\n%s", got)
	}

	var out, status bytes.Buffer
	if err := renderChatText(&out, &status, doc, citationModeList, ctx); err != nil {
		t.Fatalf("renderChatText: %v", err)
	}
	if got := status.String(); strings.Contains(got, "unavailable") {
		t.Errorf("no hook: text must not print a title hint; got:\n%s", got)
	}
}

// A citation's SourceID is a granular chunk handle that is absent from the
// source list; its ParentSourceID is the notebook source that resolves to a
// title. citationSourceID and citationTitle must prefer the parent, falling
// back to SourceID only when no parent was embedded.
func TestCitationParentSourcePreferred(t *testing.T) {
	const (
		chunk  = "cccccccc-1111-2222-3333-444444444444"
		parent = "11111111-2222-3333-4444-555555555555"
	)
	// resolveTitle only knows the PARENT — mirroring live, where the chunk id
	// never appears in the source list.
	resolveTitle := func(id string) string {
		if id == parent {
			return "product-docs.md"
		}
		return ""
	}

	// citationSourceID prefers the parent, else the chunk.
	if got := citationSourceID(api.Citation{SourceID: chunk, ParentSourceID: parent}); got != parent {
		t.Errorf("citationSourceID with parent = %q, want %q", got, parent)
	}
	if got := citationSourceID(api.Citation{SourceID: chunk}); got != chunk {
		t.Errorf("citationSourceID without parent = %q, want %q", got, chunk)
	}

	// citationTitle resolves via the parent even though the chunk id would miss.
	if got := citationTitle(api.Citation{SourceID: chunk, ParentSourceID: parent}, resolveTitle); got != "product-docs.md" {
		t.Errorf("citationTitle = %q, want product-docs.md", got)
	}
	// No parent: the chunk id misses, so no title — the pre-§9 behavior.
	if got := citationTitle(api.Citation{SourceID: chunk}, resolveTitle); got != "" {
		t.Errorf("citationTitle without parent = %q, want empty", got)
	}
}

// The core §9 behavior: persistableCitations bakes the title at save time by
// resolving the PARENT source, so a replayed session renders a real title
// offline even though the persisted SourceID (a chunk) never resolves.
func TestPersistableCitationsBakesParentTitle(t *testing.T) {
	const (
		chunk  = "cccccccc-1111-2222-3333-444444444444"
		parent = "11111111-2222-3333-4444-555555555555"
	)
	resolveTitle := func(id string) string {
		if id == parent {
			return "product-docs.md"
		}
		return "" // the chunk id is not in the source list
	}
	cites := []api.Citation{{SourceIndex: 1, SourceID: chunk, ParentSourceID: parent}}

	out := persistableCitations(cites, resolveTitle)
	if len(out) != 1 {
		t.Fatalf("got %d citations, want 1", len(out))
	}
	if out[0].Title != "product-docs.md" {
		t.Errorf("baked Title = %q, want product-docs.md (resolved via parent)", out[0].Title)
	}
	// The chunk-level SourceID must survive unchanged — we bake a title, not
	// rewrite the id.
	if out[0].SourceID != chunk || out[0].ParentSourceID != parent {
		t.Errorf("ids mutated: SourceID=%q ParentSourceID=%q", out[0].SourceID, out[0].ParentSourceID)
	}
}

// A citation whose PARENT resolves to a title must NOT read as unresolved, even
// though its chunk-level SourceID is absent from the source list. This is the
// case §9 fixes: pre-fix, the chunk id missed and every citation showed the
// title-unavailable hint; now the parent resolves it.
func TestCitationResolvableParentNotUnresolved(t *testing.T) {
	const (
		chunk  = "cccccccc-1111-2222-3333-444444444444"
		parent = "11111111-2222-3333-4444-555555555555"
	)
	ctx := chatRenderContext{
		resolveTitle: func(id string) string {
			if id == parent {
				return "product-docs.md"
			}
			return ""
		},
		// The chunk id is absent from the list; the parent is present.
		sourceRemoved: func(id string) bool { return id != parent },
	}
	c := api.Citation{SourceIndex: 1, SourceID: chunk, ParentSourceID: parent}

	if got := ctx.citationSourceTitle(c); got != "product-docs.md" {
		t.Errorf("citationSourceTitle = %q, want product-docs.md", got)
	}
	if ctx.citationSourceRemoved(c) {
		t.Errorf("citation with a resolvable parent must not read as unresolved")
	}
}

// §9 persists the baked title AND the parent id so a replayed session renders a
// real title offline. Pin that both survive a ChatMessage JSON round-trip — if
// ParentSourceID were dropped, a re-fetch would fall back to the chunk id and
// the title would collapse again on the next open.
func TestChatMessageCitationParentRoundTrip(t *testing.T) {
	const (
		chunk  = "cccccccc-1111-2222-3333-444444444444"
		parent = "11111111-2222-3333-4444-555555555555"
	)
	msg := ChatMessage{
		Role:    "assistant",
		Content: "Grounded claim. [1]",
		Citations: []api.Citation{
			{SourceIndex: 1, SourceID: chunk, ParentSourceID: parent, Title: "product-docs.md", Confidence: 0.9},
		},
	}

	blob, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back ChatMessage
	if err := json.Unmarshal(blob, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(back.Citations) != 1 {
		t.Fatalf("got %d citations, want 1", len(back.Citations))
	}
	got := back.Citations[0]
	if got.ParentSourceID != parent {
		t.Errorf("ParentSourceID lost in round-trip: got %q, want %q", got.ParentSourceID, parent)
	}
	if got.SourceID != chunk {
		t.Errorf("SourceID = %q, want %q", got.SourceID, chunk)
	}
	if got.Title != "product-docs.md" {
		t.Errorf("Title = %q, want product-docs.md", got.Title)
	}
}
