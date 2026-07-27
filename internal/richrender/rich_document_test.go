package richrender

import (
	"strings"
	"testing"
)

// dirtyTree builds a synthetic rich document shaped like the real wire decode,
// deliberately tripping all three gotchas the peer measured on the committed
// frame so the projection is tested against them rather than a cleaned-up model:
//
//   - Gotcha 1: blocks are OUT OF DOCUMENT ORDER. The second block (start 40)
//     precedes the first (start 0) in the slice; a correct projection sorts by
//     start. Rendering in slice order would scramble the document.
//   - Gotcha 2: offsets are STRINGS ("0","40",…), the beprotojson int64-as-string
//     artifact; they must be parsed before any compare/slice.
//   - Gotcha 3: real text lives in NESTED SpanGroup children, not top-level
//     blocks — the first paragraph's words are two child leaves under a group, so
//     a flatten that stops at the top level undercounts and desyncs.
//
// Document (in reading order after sorting): a paragraph "Hello world" [0,11), a
// separator [11,12) (the paragraph break), a heading-marked run "Notes" [12,17)
// (heading is an INLINE flag1 mark, not a block kind), and a two-item list
// [17,40). Offsets are contiguous so the containment test can assert full
// coverage. List membership is structural — the item groups carry a ListItem —
// not a "list"/"listItem" Kind string, which does not exist on the wire.
func dirtyTree() *RichDocument {
	return &RichDocument{
		Blocks: []richSpan{
			// Out of order on purpose: the list (start 17) appears before the
			// paragraph (start 0) and heading run (start 12).
			{Start: "17", End: "40", Group: &richGroup{Children: []richSpan{
				{Start: "17", End: "27", Group: &richGroup{ListItem: &richListItem{Nesting: 0, Bullet: "•"}, Children: []richSpan{
					{Start: "17", End: "27", Leaf: &richLeaf{Text: "first item"}},
				}}},
				{Start: "27", End: "40", Group: &richGroup{ListItem: &richListItem{Nesting: 1, Bullet: "◦"}, Children: []richSpan{
					{Start: "27", End: "40", Leaf: &richLeaf{Text: "second item!"}},
				}}},
			}}},
			// Paragraph with NESTED children: "Hello" + " world" as two leaves
			// under a group, so the flatten must recurse to see the full text.
			// "Hello" is flag1 (generic emphasis / heading hint), " world" flag8
			// (inline code).
			{Start: "0", End: "11", Group: &richGroup{Children: []richSpan{
				{Start: "0", End: "5", Leaf: &richLeaf{Text: "Hello", Marks: &richMarks{Flag1: true}}},
				{Start: "5", End: "11", Leaf: &richLeaf{Text: " world", Marks: &richMarks{Flag8: true}}},
			}}},
			// Zero-content separator: the paragraph boundary.
			{Start: "11", End: "12", Separator: true},
			// A heading-marked run: flag1 rides on the leaf; there is no heading
			// block kind, so this projects to a paragraph whose run is Heading.
			{Start: "12", End: "17", Group: &richGroup{Children: []richSpan{
				{Start: "12", End: "17", Leaf: &richLeaf{Text: "Notes", Marks: &richMarks{Flag1: true}}},
			}}},
		},
	}
}

func TestProjectRichDocumentSortsByStart(t *testing.T) {
	blocks := projectRichDocument(dirtyTree())
	if len(blocks) != 4 {
		t.Fatalf("got %d blocks, want 4", len(blocks))
	}
	// The heading run is a paragraph block (heading has no block kind — it's an
	// inline mark), so the sorted sequence is paragraph, separator, paragraph,
	// list.
	wantKinds := []blockKind{blockParagraph, blockSeparator, blockParagraph, blockList}
	for i, want := range wantKinds {
		if blocks[i].Kind != want {
			t.Errorf("block %d: kind %v, want %v", i, blocks[i].Kind, want)
		}
	}
	// Reading order is strictly ascending by start after the sort.
	for i := 1; i < len(blocks); i++ {
		if blocks[i].Start < blocks[i-1].Start {
			t.Errorf("blocks not sorted: block %d start %d < block %d start %d",
				i, blocks[i].Start, i-1, blocks[i-1].Start)
		}
	}
}

func TestProjectRichDocumentRecursiveFlatten(t *testing.T) {
	blocks := projectRichDocument(dirtyTree())
	// The paragraph's text is in two nested leaves; a non-recursive flatten
	// would miss " world".
	para := blocks[0]
	if got := runsText(para.Runs); got != "Hello world" {
		t.Errorf("paragraph text = %q, want %q (recursive flatten missed nested leaf?)", got, "Hello world")
	}
	if len(para.Runs) != 2 {
		t.Fatalf("paragraph runs = %d, want 2", len(para.Runs))
	}
	// Marks fold to the safe subset: flag1 -> emphasis + heading hint (no code);
	// flag8 -> code (and emphasis stays off).
	if !para.Runs[0].Emphasis || !para.Runs[0].Heading || para.Runs[0].Code {
		t.Errorf("run 0 marks = %+v, want emphasis+heading, no code", para.Runs[0])
	}
	if para.Runs[1].Emphasis || para.Runs[1].Heading || !para.Runs[1].Code {
		t.Errorf("run 1 marks = %+v, want code only", para.Runs[1])
	}
}

func TestProjectRichDocumentList(t *testing.T) {
	blocks := projectRichDocument(dirtyTree())
	list := blocks[3]
	if list.Kind != blockList {
		t.Fatalf("block 3 kind = %v, want blockList", list.Kind)
	}
	if len(list.Items) != 2 {
		t.Fatalf("list items = %d, want 2", len(list.Items))
	}
	if got := runsText(list.Items[0].Runs); got != "first item" {
		t.Errorf("item 0 = %q, want %q", got, "first item")
	}
	if got := runsText(list.Items[1].Runs); got != "second item!" {
		t.Errorf("item 1 = %q, want %q", got, "second item!")
	}
	// Nesting depth carries through from the wire ListItem.
	if list.Items[0].Nesting != 0 || list.Items[1].Nesting != 1 {
		t.Errorf("nesting = [%d,%d], want [0,1]", list.Items[0].Nesting, list.Items[1].Nesting)
	}
}

// TestProjectRichDocumentSiblingListCoalesce covers the real wire shape: the list
// items are TOP-LEVEL sibling spans (each a group with its own ListItem),
// interleaved with paragraphs — not children of one group. projectRichDocument
// must coalesce a maximal run of consecutive item-spans into ONE blockList, and a
// paragraph between two runs must split them into two lists.
func TestProjectRichDocumentSiblingListCoalesce(t *testing.T) {
	item := func(start, end, text string, nesting int) richSpan {
		return richSpan{Start: start, End: end, Group: &richGroup{
			ListItem: &richListItem{Nesting: nesting, Bullet: "•"},
			Children: []richSpan{{Start: start, End: end, Leaf: &richLeaf{Text: text}}},
		}}
	}
	para := func(start, end, text string) richSpan {
		return richSpan{Start: start, End: end, Group: &richGroup{
			Children: []richSpan{{Start: start, End: end, Leaf: &richLeaf{Text: text}}},
		}}
	}
	// para, item, item (one list), para (splits), item (a second, separate list).
	doc := &RichDocument{Blocks: []richSpan{
		para("0", "6", "Intro:"),
		item("6", "11", "aaaaa", 0),
		item("11", "16", "bbbbb", 1),
		para("16", "22", "Break:"),
		item("22", "27", "ccccc", 0),
	}}
	blocks := projectRichDocument(doc)

	var kinds []blockKind
	for _, b := range blocks {
		kinds = append(kinds, b.Kind)
	}
	want := []blockKind{blockParagraph, blockList, blockParagraph, blockList}
	if len(kinds) != len(want) {
		t.Fatalf("block kinds = %v, want %v (para, one merged list, para, second list)", kinds, want)
	}
	for i := range want {
		if kinds[i] != want[i] {
			t.Fatalf("block[%d] kind = %v, want %v (kinds=%v)", i, kinds[i], want[i], kinds)
		}
	}
	// The first list merged its two adjacent items; nesting carried per item.
	if got := blocks[1]; len(got.Items) != 2 || got.Items[0].Nesting != 0 || got.Items[1].Nesting != 1 {
		t.Errorf("first list items = %+v, want 2 items nesting [0,1]", got.Items)
	}
	// The paragraph between the runs kept the second item in its own list.
	if got := blocks[3]; len(got.Items) != 1 {
		t.Errorf("second list items = %d, want 1 (paragraph must close the first run)", len(got.Items))
	}
}

// The offsets are STRINGS on the wire; parseOffset must turn them into the ints
// the sort and coverage compare on. A missing offset is the document start (0),
// not a parse failure.
func TestParseOffset(t *testing.T) {
	cases := map[string]int{"0": 0, "115": 115, "": 0, "notanint": 0, "959": 959}
	for in, want := range cases {
		if got := parseOffset(in); got != want {
			t.Errorf("parseOffset(%q) = %d, want %d", in, got, want)
		}
	}
}

// flattenText reinserts the paragraph breaks the server stripped: blocks joined
// by a blank line, list items as "- " lines. This is the text/TUI floor that
// undoes the run-together artifact.
func TestFlattenTextReinsertsBreaks(t *testing.T) {
	got := flattenText(projectRichDocument(dirtyTree()))
	// The nested second item (nesting 1) is indented; the heading run is plain
	// text at the block level.
	want := "Hello world\n\nNotes\n\n- first item\n\n  - second item!"
	if got != want {
		t.Errorf("flattenText:\n got %q\nwant %q", got, want)
	}
	// The whole point: the run-together artifact is gone — there IS a break
	// between the paragraph and the heading.
	if !strings.Contains(got, "world\n\nNotes") {
		t.Errorf("paragraph break not reinserted between paragraph and heading:\n%s", got)
	}
}

// The core alignment invariant: a citation reply_span keyed by wire offsets is
// covered by a contiguous run of the recursively-flattened leaf spans. This is
// what lets the existing [N] hovercards layer onto tree-rendered output — the
// rich spans and the citation ranges share one offset space (verified on the
// real frame: block_max_end == annotation_max_end). We assert containment, NOT
// len(flatten) == len(Content): the tree's rendered length need not equal the
// flat string; only the offsets must line up.
func TestOffsetContainment(t *testing.T) {
	leaves := leafSpans(dirtyTree())

	// The document's leaves cover [0,40) contiguously (the separator at [11,12)
	// carries no leaf, so a citation must not claim to cover across it unless a
	// leaf does — here the heading leaf starts at 12, leaving [11,12) uncovered).
	covered := []struct{ s, e int }{
		{0, 11},  // the paragraph, spanning two nested leaves
		{0, 5},   // just "Hello"
		{5, 11},  // just " world"
		{12, 17}, // the heading
		{17, 40}, // the whole list
		{17, 27}, // first item
	}
	for _, c := range covered {
		if !spanCovered(leaves, c.s, c.e) {
			t.Errorf("span [%d,%d) should be covered by leaf spans", c.s, c.e)
		}
	}

	// A range crossing the separator gap [11,12) is NOT contiguously covered —
	// there is no leaf there. A citation landing here would be a real desync.
	if spanCovered(leaves, 5, 15) {
		t.Errorf("span [5,15) crosses the uncovered separator gap; should not be covered")
	}
	// A range past the document end is not covered.
	if spanCovered(leaves, 30, 100) {
		t.Errorf("span [30,100) runs past document end; should not be covered")
	}
}

// A nil tree or an empty one yields no projected blocks, so a caller falls back
// to flat Content — the progressive-enhancement floor. leafSpans and flattenText
// must be nil-safe for the same reason.
func TestRichDocumentEmptyFallback(t *testing.T) {
	if got := projectRichDocument(nil); got != nil {
		t.Errorf("projectRichDocument(nil) = %v, want nil", got)
	}
	if got := projectRichDocument(&RichDocument{}); got != nil {
		t.Errorf("projectRichDocument(empty) = %v, want nil", got)
	}
	if got := leafSpans(nil); got != nil {
		t.Errorf("leafSpans(nil) = %v, want nil", got)
	}
	if got := flattenText(nil); got != "" {
		t.Errorf("flattenText(nil) = %q, want empty", got)
	}
}

// A span with no recognized content field — a wire block type the parse layer
// couldn't classify into any modeled position — degrades to blockUnknown rather
// than a dropped or failed block (progressive-enhancement rule: unmodeled ->
// safe, never a broken render). It keeps its offsets so it still occupies its
// place in the document order.
func TestUnknownBlockDegrades(t *testing.T) {
	doc := &RichDocument{Blocks: []richSpan{
		{Start: "10", End: "20"}, // no Leaf/Group/Hidden/Table/CodeBlock/Separator
		{Start: "0", End: "10", Group: &richGroup{Children: []richSpan{
			{Start: "0", End: "10", Leaf: &richLeaf{Text: "real text"}},
		}}},
	}}
	blocks := projectRichDocument(doc)
	if len(blocks) != 2 {
		t.Fatalf("got %d blocks, want 2", len(blocks))
	}
	// Sorted: the real paragraph (start 0), then the unmodeled block (start 10).
	if blocks[0].Kind != blockParagraph || runsText(blocks[0].Runs) != "real text" {
		t.Errorf("block 0 = %+v, want paragraph 'real text'", blocks[0])
	}
	if blocks[1].Kind != blockUnknown {
		t.Errorf("block 1 kind = %v, want blockUnknown", blocks[1].Kind)
	}
	// The unmodeled block still bounds its offset range.
	if blocks[1].Start != 10 || blocks[1].End != 20 {
		t.Errorf("unknown block offsets = [%d,%d], want [10,20]", blocks[1].Start, blocks[1].End)
	}
}

// Gated table and code_block blocks degrade to flat text (blockTable /
// blockCodeBlock carrying their flattened runs) until a real frame verifies the
// structured shape — never rendered as <table>/<pre> from an unverified model.
func TestGatedTableCodeBlockDegrade(t *testing.T) {
	doc := &RichDocument{Blocks: []richSpan{
		{Start: "0", End: "10", Table: &richGroup{Children: []richSpan{
			{Start: "0", End: "10", Leaf: &richLeaf{Text: "a | b | c"}},
		}}},
		{Start: "10", End: "20", CodeBlock: &richGroup{Children: []richSpan{
			{Start: "10", End: "20", Leaf: &richLeaf{Text: "x := 1"}},
		}}},
	}}
	blocks := projectRichDocument(doc)
	if len(blocks) != 2 {
		t.Fatalf("got %d blocks, want 2", len(blocks))
	}
	if blocks[0].Kind != blockTable || runsText(blocks[0].Runs) != "a | b | c" {
		t.Errorf("block 0 = %+v, want blockTable flat text", blocks[0])
	}
	if blocks[1].Kind != blockCodeBlock || runsText(blocks[1].Runs) != "x := 1" {
		t.Errorf("block 1 = %+v, want blockCodeBlock flat text", blocks[1])
	}
}

// Hidden content (thinking/reasoning blocks) projects to blockHidden with its
// text flattened; the renderers show it only when the caller opts in.
func TestHiddenBlockProjects(t *testing.T) {
	doc := &RichDocument{Blocks: []richSpan{
		{Start: "0", End: "20", Hidden: &richGroup{Children: []richSpan{
			{Start: "0", End: "20", Leaf: &richLeaf{Text: "reasoning trace"}},
		}}},
	}}
	blocks := projectRichDocument(doc)
	if len(blocks) != 1 || blocks[0].Kind != blockHidden {
		t.Fatalf("got %+v, want one blockHidden", blocks)
	}
	if got := runsText(blocks[0].Runs); got != "reasoning trace" {
		t.Errorf("hidden text = %q, want %q", got, "reasoning trace")
	}
	// Hidden content is excluded from the containment leaf set (it is not part
	// of the visible answer's offset space).
	if leaves := leafSpans(doc); len(leaves) != 0 {
		t.Errorf("hidden-only doc should contribute no visible leaves, got %d", len(leaves))
	}
}

// HAZARD (peer-flagged, must be re-verified on the real fixture): in the real
// frame hidden_content blocks carry offsets INTERLEAVED in the same numeric
// range as visible content, not appended. leafSpans excludes hidden ranges, so
// a visible block AFTER a hidden block starts at the offset the hidden block
// ended — i.e. the visible offset space has a GAP where the hidden block sits.
// This test pins the current assumption: a citation whose reply_span lands on
// the post-hidden visible block is covered, and a span reaching back INTO the
// hidden range is NOT (hidden is not citable visible text).
//
// If the real fixture shows citation reply_spans instead index a space that
// INCLUDES hidden ranges, this assumption is wrong and leafSpans must emit a
// placeholder (offset-only, no text) for hidden blocks so the coordinate space
// stays contiguous. Do not delete this test when the fixture lands — flip it to
// whichever behavior the real offsets prove.
func TestHiddenBlockOffsetGap(t *testing.T) {
	// Visible "intro" [0,10), hidden reasoning [10,50), visible "after" [50,60).
	doc := &RichDocument{Blocks: []richSpan{
		{Start: "0", End: "10", Group: &richGroup{Children: []richSpan{
			{Start: "0", End: "10", Leaf: &richLeaf{Text: "intro text"}},
		}}},
		{Start: "10", End: "50", Hidden: &richGroup{Children: []richSpan{
			{Start: "10", End: "50", Leaf: &richLeaf{Text: "long hidden reasoning trace"}},
		}}},
		{Start: "50", End: "60", Group: &richGroup{Children: []richSpan{
			{Start: "50", End: "60", Leaf: &richLeaf{Text: "after text"}},
		}}},
	}}
	leaves := leafSpans(doc)

	// A citation on the intro (before the hidden block) is covered.
	if !spanCovered(leaves, 0, 10) {
		t.Errorf("citation on intro [0,10) should be covered")
	}
	// A citation on the post-hidden visible block, keyed by its OWN wire offsets
	// [50,60), is covered — this is the peer's "citation after a hidden block"
	// case, and it lands correctly because we index by wire offset, not by a
	// recomputed length that would be short by the hidden block's 40 chars.
	if !spanCovered(leaves, 50, 60) {
		t.Errorf("citation on post-hidden block [50,60) should be covered by wire offset")
	}
	// A span reaching into the hidden range is NOT contiguously covered: there is
	// no visible leaf at [10,50).
	if spanCovered(leaves, 5, 55) {
		t.Errorf("span crossing the hidden gap [10,50) should not be covered")
	}
}
