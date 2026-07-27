package main

import (
	"strings"
	"testing"

	"github.com/tmc/nlm/internal/notebooklm/api"
)

// TestRichDocumentFromAPI maps a small api tree to the render model and checks
// the structural cases: a leaf keeps its text and marks, a group keeps its
// children, and a bare span becomes a separator.
func TestRichDocumentFromAPI(t *testing.T) {
	rc := &api.RichContent{Blocks: []api.RichSpan{
		{Start: 0, End: 5, Children: []api.RichSpan{
			{Start: 0, End: 5, Text: "Hello"},
		}},
		{Start: 5, End: 6}, // bare → separator
		{Start: 6, End: 10, Children: []api.RichSpan{
			{Start: 6, End: 10, Text: "code", Marks: []bool{false, false, false, false, false, false, false, true}},
		}},
	}}

	doc := richDocumentFromAPI(rc)
	if doc == nil || len(doc.Blocks) != 3 {
		t.Fatalf("want 3 blocks, got %+v", doc)
	}
	// Block 0: group with one leaf "Hello".
	if doc.Blocks[0].Group == nil || len(doc.Blocks[0].Group.Children) != 1 {
		t.Fatalf("block0 should be a group with one child: %+v", doc.Blocks[0])
	}
	leaf := doc.Blocks[0].Group.Children[0].Leaf
	if leaf == nil || leaf.Text != "Hello" {
		t.Errorf("block0 child leaf = %+v, want text Hello", leaf)
	}
	// Block 1: separator.
	if !doc.Blocks[1].Separator {
		t.Errorf("block1 should be a separator: %+v", doc.Blocks[1])
	}
	// Block 2: group whose leaf carries the code mark (position 7 → Flag8).
	codeLeaf := doc.Blocks[2].Group.Children[0].Leaf
	if codeLeaf == nil || codeLeaf.Marks == nil || !codeLeaf.Marks.Flag8 {
		t.Errorf("block2 leaf should carry Flag8 (code): %+v", codeLeaf)
	}
}

// TestRichReflowIntegrity is the (c) reflow check: bridge a multi-block tree,
// project it, and flatten to text — the reflowed body must (1) reinsert the
// paragraph breaks the flat text lacks and (2) preserve every non-whitespace
// character (offset integrity: reflow only adds whitespace, never drops or
// reorders content, so citation offsets into the flat text stay valid). The
// real-frame decode is covered in the api package
// (TestDecodeRichContentRealFrame); this exercises the bridge→project→flatten
// chain that runs on cmd side.
func TestRichReflowIntegrity(t *testing.T) {
	// Three paragraphs the flat text would run together ("AlphaBetaGamma").
	rc := &api.RichContent{Blocks: []api.RichSpan{
		{Start: 0, End: 5, Children: []api.RichSpan{{Start: 0, End: 5, Text: "Alpha"}}},
		{Start: 5, End: 9, Children: []api.RichSpan{{Start: 5, End: 9, Text: "Beta"}}},
		{Start: 9, End: 14, Children: []api.RichSpan{{Start: 9, End: 14, Text: "Gamma"}}},
	}}
	doc := richDocumentFromAPI(rc)
	if doc == nil {
		t.Fatal("bridge returned nil")
	}
	reflowed := flattenText(projectRichDocument(doc))

	if !strings.Contains(reflowed, "\n\n") {
		t.Errorf("reflowed body has no paragraph break — structure not reconstructed: %q", reflowed)
	}
	flat := flattenLeafText(doc)
	if strip(reflowed) != strip(flat) {
		t.Errorf("reflow changed non-whitespace content:\n flat: %q\n refl: %q",
			trunc(strip(flat)), trunc(strip(reflowed)))
	}
	if strip(flat) != "AlphaBetaGamma" {
		t.Errorf("flat leaf text = %q, want AlphaBetaGamma", strip(flat))
	}
}

// flattenLeafText concatenates every leaf's text in document order with no
// separators — the flat baseline the reflow must preserve character-for-character
// (modulo whitespace).
func flattenLeafText(doc *richDocument) string {
	var b strings.Builder
	var walk func(s richSpan)
	walk = func(s richSpan) {
		if s.Leaf != nil {
			b.WriteString(s.Leaf.Text)
		}
		if s.Group != nil {
			for _, c := range s.Group.Children {
				walk(c)
			}
		}
	}
	for _, blk := range doc.Blocks {
		walk(blk)
	}
	return b.String()
}

func strip(s string) string { return strings.Join(strings.Fields(s), "") }

func trunc(s string) string {
	if len(s) > 80 {
		return s[:80] + "…"
	}
	return s
}
