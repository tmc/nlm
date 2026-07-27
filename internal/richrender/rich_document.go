package richrender

// Rich-document scaffolding for the answer body.
//
// The NotebookLM server ships answer text with every newline stripped: on the
// wire a turn's whole answer is one run of newline-free strings, and ALL
// document structure — paragraph breaks, lists, emphasis — lives in a span tree
// alongside that text, not in the text itself. The flat renderers therefore
// cannot reconstruct paragraphs (there are no \n to split on); that is the
// run-together artifact ("harnessImplemented", "notes2026-07-09"). Rendering
// from the tree is the fix.
//
// This file is the branch-local model + projection ONLY. It is deliberately
// scaffolding ahead of the parse layer: the real RichDocument proto and its
// decode into api.ChatMessage land separately (steps a/b, proto/betool turf).
// Nothing here touches the live render path — a message carries a *richDocument
// only when that parse layer populates it, and every renderer keeps flat
// Content as the floor (renderRich* is only reached when Rich != nil).
//
// The model mirrors the WIRE-DECODED shape, not a cleaned-up one, on purpose:
//   - offsets arrive as STRINGS ("0","115"), a beprotojson int64-as-string
//     artifact — richSpan.Start/End are strings, parsed on projection;
//   - blocks are NOT in document order — they must be sorted by start;
//   - a block's real content lives in NESTED child spans, so flattening must
//     recurse — a top-level-blocks-only walk undercounts and desyncs.
// projectRichDocument handles all three; the tests exercise a stub built to
// trip each one (see rich_document_test.go). Modeling the stub already-clean
// would hide the gotchas until real data merged.

import (
	"sort"
	"strconv"
	"strings"
)

// richDocument is the wire-decoded answer body: the block tree (paragraphs,
// list items, separators, and gated table/code blocks) over the newline-free
// reply text. It is the parsed form of the RichDocument proto's body layer;
// annotations (the marker→source offset index) stay with the citation model and
// are not duplicated here. A nil *richDocument means "no tree" — render flat
// Content.
type richDocument struct {
	Blocks []richSpan
}

// richSpan is one character-offset range of the rendered document. Spans nest:
// a block span's content is a group of child spans; a leaf span's content is
// text. Start/End are wire strings ("0","115") parsed to int on projection so a
// legitimate 0 (document start) is a real offset, not an absent value.
//
// A span's TYPE is structural — which content field is populated, mirroring the
// proto's field positions — not a label. At most one of Leaf, Group, Hidden,
// Table, or CodeBlock is set; Separator is a bare boundary marker. Table and
// CodeBlock are modeled but gated (rendered flat) until a real frame verifies
// their shape.
type richSpan struct {
	Start     string // wire offset, e.g. "115"; "" when absent
	End       string
	Leaf      *richLeaf
	Group     *richGroup // content (proto field 3)
	Hidden    *richGroup // thinking block content (proto hidden_content, field 9)
	Table     *richGroup // gated: table content (proto field 5); rendered flat
	CodeBlock *richGroup // gated: code-block content (proto field 7); rendered flat
	Separator bool       // a zero-width block boundary (proto separator, field 12)
}

// richLeaf is the innermost content: a run of text with optional formatting
// marks. Marks are the positional TextMarks flags; only the ones we can render
// safely are acted on (see leafRuns).
type richLeaf struct {
	Text  string
	Marks *richMarks
}

// richGroup is a container span's content: child spans plus optional layout
// metadata. There is NO block-kind string on the wire — a block's type is
// structural. A content group IS a list when it carries a ListItem (proto
// SpanGroup.list_item, field 4); otherwise it is a paragraph. Indent is the
// group's SpanGroupMeta.indent (field 2); ListItem carries per-item nesting and
// bullet style. Both are nil/zero on an ordinary paragraph group.
type richGroup struct {
	Children []richSpan
	Indent   int           // SpanGroupMeta.indent; 0 when absent
	ListItem *richListItem // present → this group is a list item
}

// richListItem is the list-item style on a content group: nesting depth (0/1/2)
// and the bullet glyph. Its presence — not any Kind label — is what marks a
// group as a list.
type richListItem struct {
	Nesting int    // depth 0/1/2
	Bullet  string // "•"/"◦"/"▪" or a numeric style; "" when unknown
}

// richMarks are the positional formatting flags on a text leaf (proto
// TextMarks). Only Link and Flag8 are asserted (Link is a confirmed field;
// Flag8 is observed on code/identifier runs → inline code). Flag1 is observed
// on inline headings/labels and doubles as generic emphasis — both unconfirmed,
// so it renders as generic emphasis and, optionally, a heading hint. The rest
// fold to generic emphasis rather than an unverified bold-vs-italic split.
type richMarks struct {
	Flag1 bool   // inline heading/label OR generic emphasis (unconfirmed)
	Flag2 bool   // generic emphasis (unconfirmed)
	Flag8 bool   // inline code / identifier run
	Link  string // href, "" when none (confirmed proto field 4)
}

// Projected model: the clean, ordered, typed output the renderers consume.
// projectRichDocument turns the dirty wire model into this.

// blockKind is the rendered class of a projected block. The kinds mirror the
// wire's STRUCTURAL block types (which Span field is populated), not a label:
// a content span is a paragraph or, if its group carries a list item, a list;
// hidden_content is a reasoning block; separator is a boundary; table and
// code_block are gated (rendered as flat text) until a real frame verifies them.
// There is deliberately no blockHeading — heading has no wire block encoding; it
// rides on an inline mark (see richRun.Heading).
type blockKind int

const (
	blockParagraph blockKind = iota
	blockList
	blockSeparator // a paragraph boundary; carries no text
	blockHidden    // thinking/reasoning; renderers show it only when opted in
	blockTable     // gated: rendered as flat text until verified
	blockCodeBlock // gated: rendered as flat text until verified
	blockUnknown   // unmodeled: render its flat text slice, never fail
)

// richBlockOut is one projected block in reading order: its kind and the inline
// runs (for list blocks, Items holds the per-item run lists). Renderers project
// this to <p>/<ul><li>/… (html), CommonMark (markdown), or reinserted paragraph
// breaks (text). Start/End bound the block for offset alignment.
type richBlockOut struct {
	Kind  blockKind
	Start int
	End   int
	Runs  []richRun  // inline runs for paragraph/hidden
	Items []richItem // per-item runs (+ nesting) for blockList
}

// richItem is one list item: its inline runs and nesting depth (0/1/2), so a
// renderer can indent nested items. Nesting comes from the wire ListItem.
type richItem struct {
	Runs    []richRun
	Nesting int
}

// richRun is one inline run of text with its (safe) formatting. Emphasis is the
// generic flag fold; Code (flag8) and Link are the asserted marks. Heading is
// the flag1 "inline heading/label" hint — unconfirmed, so renderers may surface
// it as a heading or fall back to emphasis.
type richRun struct {
	Text     string
	Emphasis bool
	Code     bool
	Heading  bool
	Link     string
	Start    int
	End      int
}

// projectRichDocument turns the wire-decoded tree into ordered, typed blocks.
// It handles the three wire gotchas in one place:
//   - offsets are strings → parsed to int (a bad/empty offset sorts as 0 but
//     still renders; alignment tests catch a real desync);
//   - blocks are unsorted → sorted by start for reading order;
//   - content is nested → flattened recursively.
//
// A nil document or no blocks yields nil, so the caller falls back to flat
// Content. An unmodeled block becomes a blockUnknown carrying its flattened
// text, never a dropped or failed block.
func projectRichDocument(doc *richDocument) []richBlockOut {
	if doc == nil || len(doc.Blocks) == 0 {
		return nil
	}
	blocks := make([]richSpan, len(doc.Blocks))
	copy(blocks, doc.Blocks)
	sortSpansByStart(blocks)

	// On the wire the items of a list are TOP-LEVEL sibling spans (each a group
	// carrying its own ListItem), interleaved with paragraphs — not children of a
	// single list group. So coalesce here: a maximal run of consecutive list-item
	// spans becomes one blockList; any non-item block between two items closes the
	// run. Adjacency is the boundary (a paragraph — e.g. a section heading —
	// separates two lists); the wire's per-item sequence counter is not consulted.
	out := make([]richBlockOut, 0, len(blocks))
	for i := 0; i < len(blocks); {
		if isListItemSpan(blocks[i]) {
			j := i + 1
			for j < len(blocks) && isListItemSpan(blocks[j]) {
				j++
			}
			out = append(out, mergeListItems(blocks[i:j]))
			i = j
			continue
		}
		out = append(out, projectBlock(blocks[i]))
		i++
	}
	return out
}

// isListItemSpan reports whether a top-level span is a list item — a content
// group carrying a ListItem marker. Consecutive such spans coalesce into one
// list.
func isListItemSpan(b richSpan) bool {
	return b.Group != nil && b.Group.ListItem != nil
}

// mergeListItems builds one blockList from a run of consecutive list-item spans,
// one richItem per span (its flattened runs and nesting depth). Start/End span
// the whole run so the list block's offsets bound its items for alignment.
func mergeListItems(spans []richSpan) richBlockOut {
	items := make([]richItem, 0, len(spans))
	start := parseOffset(spans[0].Start)
	end := parseOffset(spans[len(spans)-1].End)
	for _, s := range spans {
		items = append(items, richItem{
			Runs:    flattenRuns(s.Group.Children),
			Nesting: s.Group.ListItem.Nesting,
		})
	}
	return richBlockOut{Kind: blockList, Start: start, End: end, Items: items}
}

// sortSpansByStart orders spans by parsed start offset, stably so equal-start
// spans keep wire order. This is the fix for blocks arriving out of document
// order (a block at 1143 can precede one at 115 on the wire).
func sortSpansByStart(spans []richSpan) {
	sort.SliceStable(spans, func(i, j int) bool {
		return parseOffset(spans[i].Start) < parseOffset(spans[j].Start)
	})
}

// parseOffset parses a wire offset string to int. An empty or malformed offset
// becomes 0 — the document start — which is a safe sort key; a genuine
// misalignment surfaces in the offset-containment test, not as a parse panic.
func parseOffset(s string) int {
	if s == "" {
		return 0
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}

// projectBlock classifies one top-level span into a projected block. Separators
// become blockSeparator (the paragraph boundary); hidden content becomes
// blockHidden; a group is classified by its Kind; anything else degrades to
// blockUnknown carrying its flattened text.
func projectBlock(b richSpan) richBlockOut {
	start, end := parseOffset(b.Start), parseOffset(b.End)
	switch {
	case b.Separator:
		return richBlockOut{Kind: blockSeparator, Start: start, End: end}
	case b.Hidden != nil:
		return richBlockOut{Kind: blockHidden, Start: start, End: end, Runs: flattenRuns(b.Hidden.Children)}
	case b.Table != nil:
		// Gated: table structure is unverified, so degrade to its flat text.
		return richBlockOut{Kind: blockTable, Start: start, End: end, Runs: flattenRuns(b.Table.Children)}
	case b.CodeBlock != nil:
		// Gated: code-block structure is unverified, so degrade to its flat text.
		return richBlockOut{Kind: blockCodeBlock, Start: start, End: end, Runs: flattenRuns(b.CodeBlock.Children)}
	case b.Group != nil:
		return projectContentBlock(b.Group, start, end)
	case b.Leaf != nil:
		return richBlockOut{Kind: blockParagraph, Start: start, End: end, Runs: leafRuns(b, start, end)}
	default:
		// A present-but-empty or unmodeled block: no content to render.
		return richBlockOut{Kind: blockUnknown, Start: start, End: end}
	}
}

// projectContentBlock classifies a content (field-3) group STRUCTURALLY — there
// is no Kind label on the wire. A group whose children carry list items is a
// list; each list-item child becomes a richItem with its nesting depth.
// Otherwise it is a paragraph and its children flatten to one run list.
func projectContentBlock(g *richGroup, start, end int) richBlockOut {
	if items := listItems(g); items != nil {
		return richBlockOut{Kind: blockList, Start: start, End: end, Items: items}
	}
	return richBlockOut{Kind: blockParagraph, Start: start, End: end, Runs: flattenRuns(g.Children)}
}

// listItems collects a group's list-item children into per-item runs, or
// returns nil when the group is not a list. A child is a list item when its own
// group carries a ListItem (proto SpanGroup.list_item); the item's nesting comes
// from that ListItem. A group that is itself directly a single list item (its
// own ListItem set) is treated as a one-item list so a lone bullet still renders.
func listItems(g *richGroup) []richItem {
	if g.ListItem != nil {
		return []richItem{{Runs: flattenRuns(g.Children), Nesting: g.ListItem.Nesting}}
	}
	var items []richItem
	for _, child := range g.Children {
		if child.Group != nil && child.Group.ListItem != nil {
			items = append(items, richItem{
				Runs:    flattenRuns(child.Group.Children),
				Nesting: child.Group.ListItem.Nesting,
			})
		}
	}
	return items
}

// flattenRuns walks a list of child spans in order and concatenates their leaf
// runs, recursing through nested groups. This is the recursive flatten the wire
// requires: a block's real text is in nested SpanGroup children, so a walk that
// stops at the top level undercounts and desyncs from the citation offsets.
func flattenRuns(spans []richSpan) []richRun {
	var runs []richRun
	for _, s := range spans {
		runs = append(runs, flattenSpanRuns(s)...)
	}
	return runs
}

// flattenSpanRuns flattens a single span to its leaf runs, recursing into
// Group/Hidden children. A leaf yields one run; a group yields its children's
// runs in order.
func flattenSpanRuns(s richSpan) []richRun {
	switch {
	case s.Leaf != nil:
		return leafRuns(s, parseOffset(s.Start), parseOffset(s.End))
	case s.Group != nil:
		return flattenRuns(s.Group.Children)
	case s.Hidden != nil:
		return flattenRuns(s.Hidden.Children)
	default:
		return nil
	}
}

// leafRuns turns a leaf span into a single run, folding its marks to the safe
// subset: flag8 → Code and link → Link are asserted (both proto-confirmed);
// flag1 additionally hints an inline heading/label. Every set flag also implies
// generic Emphasis, since the exact bold/italic meaning is unconfirmed.
func leafRuns(s richSpan, start, end int) []richRun {
	if s.Leaf == nil {
		return nil
	}
	run := richRun{Text: s.Leaf.Text, Start: start, End: end}
	if m := s.Leaf.Marks; m != nil {
		run.Code = m.Flag8
		run.Heading = m.Flag1
		run.Emphasis = m.Flag1 || m.Flag2
		run.Link = m.Link
	}
	return []richRun{run}
}

// leafSpans returns every leaf run in the document in reading order (blocks
// sorted by start, children flattened recursively). It is the flattened
// coordinate space citation offsets index into: a citation reply_span [s,e]
// must be covered by a contiguous run of these leaves. Used for the
// offset-alignment check, not for rendering.
func leafSpans(doc *richDocument) []richRun {
	if doc == nil {
		return nil
	}
	blocks := make([]richSpan, len(doc.Blocks))
	copy(blocks, doc.Blocks)
	sortSpansByStart(blocks)

	var runs []richRun
	for _, b := range blocks {
		switch {
		case b.Separator:
			continue
		case b.Hidden != nil:
			// Hidden/reasoning content is not part of the visible answer's
			// offset space — citation reply_spans index the shown document — so
			// it contributes no leaves to the containment set.
			continue
		default:
			runs = append(runs, flattenSpanRuns(b)...)
		}
	}
	return runs
}

// spanCovered reports whether the character range [start,end) is covered by a
// contiguous run of leaf spans — the offset-containment invariant. A citation's
// reply_span keys into the same offset space as the rich tree (verified on the
// real frame: block_max_end == annotation_max_end), so a citation whose range
// is NOT covered signals a real desync (unsorted blocks, unparsed string
// offsets, or a non-recursive flatten) rather than a rendering choice. An empty
// or inverted range is trivially covered.
func spanCovered(leaves []richRun, start, end int) bool {
	if end <= start {
		return true
	}
	// Walk leaves in order; advance a cursor across contiguous coverage.
	cur := start
	for _, r := range leaves {
		if r.End <= cur {
			continue
		}
		if r.Start > cur {
			return false // a gap before the next leaf: not contiguous
		}
		cur = r.End
		if cur >= end {
			return true
		}
	}
	return cur >= end
}

// flattenText returns the projected blocks' text with paragraph structure
// reinserted: runs joined within a block, blocks separated by "\n\n" (a
// separator block also forces a break). This is the text/TUI floor — it undoes
// the run-together artifact without any markup. Separator and empty blocks
// contribute only their break.
func flattenText(blocks []richBlockOut) string {
	var parts []string
	for _, b := range blocks {
		switch b.Kind {
		case blockSeparator:
			continue // the join below already breaks between blocks
		case blockList:
			for _, item := range b.Items {
				indent := strings.Repeat("  ", item.Nesting)
				parts = append(parts, indent+"- "+runsText(item.Runs))
			}
		default:
			if t := runsText(b.Runs); t != "" {
				parts = append(parts, t)
			}
		}
	}
	return joinBlocks(parts)
}

// runsText concatenates a block's inline runs into its plain text.
func runsText(runs []richRun) string {
	var b []byte
	for _, r := range runs {
		b = append(b, r.Text...)
	}
	return string(b)
}

// joinBlocks joins block texts with a blank line between, the reinserted
// paragraph break the stripped newlines removed.
func joinBlocks(parts []string) string {
	return strings.Join(parts, "\n\n")
}
