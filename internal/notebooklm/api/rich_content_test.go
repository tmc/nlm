package api

import (
	"encoding/json"
	"os"
	"testing"
	"unicode/utf16"
)

// utf16CodeUnits returns the UTF-16 code-unit length of s — the unit the wire's
// answer span offsets count in (an astral rune is one rune but two units).
func utf16CodeUnits(s string) int {
	n := 0
	for _, r := range s {
		n += utf16.RuneLen(r)
	}
	return n
}

// loadRichContentBlock reads the real (synthetic-text) GetConversationHistory
// content block arr[4] captured from the khqZz frame. Its span tree lives at
// [0][4] and carries paragraph, separator, and marked-leaf blocks plus a
// grounding layer — the shapes decodeRichContent must handle and must NOT
// mistake for blocks.
func loadRichContentBlock(t *testing.T) any {
	t.Helper()
	raw, err := os.ReadFile("testdata/rich_content_block.json")
	if err != nil {
		t.Fatal(err)
	}
	var block any
	if err := json.Unmarshal(raw, &block); err != nil {
		t.Fatal(err)
	}
	return block
}

func TestDecodeRichContentRealFrame(t *testing.T) {
	rc := decodeRichContent(loadRichContentBlock(t))
	if rc == nil {
		t.Fatal("decodeRichContent returned nil on a frame that carries a tree")
	}
	if len(rc.Blocks) == 0 {
		t.Fatal("no blocks decoded")
	}

	// The block list is TREE[0]; the grounding layer at TREE[3] (entries shaped
	// [[uuid], [null, s, e]]) must NOT appear as a block. Every decoded block is
	// a real span: numeric start<=end and no source-UUID leaf text.
	for i, b := range rc.Blocks {
		if b.End < b.Start {
			t.Errorf("block[%d]: inverted span [%d,%d)", i, b.Start, b.End)
		}
		if looksLikeUUID(b.Text) {
			t.Errorf("block[%d]: grounding UUID %q leaked in as block text", i, b.Text)
		}
	}

	// Offsets must be contiguous and start at 0 — the block list covers the
	// whole answer with no gap, which is what lets a citation reply_span key
	// into the same coordinate space.
	if rc.Blocks[0].Start != 0 {
		t.Errorf("first block starts at %d, want 0", rc.Blocks[0].Start)
	}
	for i := 1; i < len(rc.Blocks); i++ {
		if rc.Blocks[i].Start != rc.Blocks[i-1].End {
			t.Errorf("gap between block[%d] end %d and block[%d] start %d",
				i-1, rc.Blocks[i-1].End, i, rc.Blocks[i].Start)
		}
	}
}

// TestDecodeRichContentLeavesAndMarks checks the leaf/group/trailer shapes the
// projection depends on: a leaf carries text, a group carries children, a
// block trailer [null,N] is captured, and an inline mark bool is decoded.
func TestDecodeRichContentLeavesAndMarks(t *testing.T) {
	rc := decodeRichContent(loadRichContentBlock(t))
	if rc == nil {
		t.Fatal("nil tree")
	}

	var sawText, sawTrailer, sawMark bool
	var walk func(s RichSpan)
	walk = func(s RichSpan) {
		if s.Text != "" {
			sawText = true
		}
		if s.HasTag {
			sawTrailer = true
		}
		for _, m := range s.Marks {
			if m {
				sawMark = true
			}
		}
		for _, c := range s.Children {
			walk(c)
		}
	}
	for _, b := range rc.Blocks {
		walk(b)
	}
	if !sawText {
		t.Error("no leaf text decoded from the tree")
	}
	if !sawTrailer {
		t.Error("no block trailer [null,N] captured (BlockTag)")
	}
	if !sawMark {
		t.Error("no inline TextMarks flag decoded")
	}
}

// TestDecodeRichContentDegrades confirms the decoder never fails: a malformed
// or tree-less block yields nil (render flat), not a panic.
func TestDecodeRichContentDegrades(t *testing.T) {
	cases := []struct {
		name  string
		block any
	}{
		{"nil", nil},
		{"empty", []any{}},
		{"flat segment (no tree)", []any{[]any{[]any{"just text"}}}},
		{"segment too short", []any{[]any{"s1", nil, nil}}},
		{"tree not a block list", []any{[]any{"s1", nil, nil, nil, []any{"scalar"}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := decodeRichContent(tc.block); got != nil {
				t.Errorf("expected nil (flat fallback), got %+v", got)
			}
		})
	}
}

// loadLiveTree reads the real (synthetic-text) GenerateFreeFormStreamed rich
// tree captured from a live chat frame's inner[0][4]. It IS the tree layer
// ([blocks, null, null, grounding, flag]) — i.e. a segment's [4] — so wrapping
// it as segment[4] exercises the same live-path entry the stream parser uses.
func loadLiveTree(t *testing.T) any {
	t.Helper()
	raw, err := os.ReadFile("testdata/live_rich_tree.json")
	if err != nil {
		t.Fatal(err)
	}
	var tree any
	if err := json.Unmarshal(raw, &tree); err != nil {
		t.Fatal(err)
	}
	return tree
}

// TestDecodeRichContentLiveSegment decodes the live wire tree through the live
// entry point (decodeRichContentFromSegment), proving the same decoder serves
// both the replay block and the live segment. The live frame carries the tree at
// inner[0][4]; here inner is ["s1", null, [ids], null, TREE].
func TestDecodeRichContentLiveSegment(t *testing.T) {
	tree := loadLiveTree(t)
	segment := []any{"s1", nil, []any{}, nil, tree}

	rc := decodeRichContentFromSegment(segment)
	if rc == nil {
		t.Fatal("decodeRichContentFromSegment returned nil on a live frame with a tree")
	}
	if len(rc.Blocks) == 0 {
		t.Fatal("no blocks decoded from the live tree")
	}

	// Same invariants as the replay frame: contiguous span coverage from 0, no
	// grounding UUID leaking in as a block, and at least one leaf and one mark.
	if rc.Blocks[0].Start != 0 {
		t.Errorf("first live block starts at %d, want 0", rc.Blocks[0].Start)
	}
	var sawText, sawMark bool
	var walk func(s RichSpan)
	walk = func(s RichSpan) {
		if s.Text != "" {
			sawText = true
		}
		if looksLikeUUID(s.Text) {
			t.Errorf("grounding UUID %q leaked in as a live block/leaf", s.Text)
		}
		for _, m := range s.Marks {
			if m {
				sawMark = true
			}
		}
		for _, c := range s.Children {
			walk(c)
		}
	}
	for i, b := range rc.Blocks {
		if b.End < b.Start {
			t.Errorf("live block[%d]: inverted span [%d,%d)", i, b.Start, b.End)
		}
		if i > 0 && b.Start != rc.Blocks[i-1].End {
			t.Errorf("gap between live block[%d] end %d and block[%d] start %d",
				i-1, rc.Blocks[i-1].End, i, b.Start)
		}
		walk(b)
	}
	if !sawText {
		t.Error("no leaf text decoded from the live tree")
	}
	if !sawMark {
		t.Error("no inline TextMarks flag decoded from the live tree")
	}
}

// TestDecodeRichContentLiveSpanIsUTF16 pins the wire's offset space: for every
// leaf, End-Start equals the text's UTF-16 code-unit length, not its rune count.
// The fixture preserves one astral leaf (📊, one rune / two UTF-16 units) whose
// span is exactly one unit wider than its rune length — the case the renderer's
// UTF-16→rune mapping must handle.
func TestDecodeRichContentLiveSpanIsUTF16(t *testing.T) {
	tree := loadLiveTree(t)
	rc := decodeRichContentFromSegment([]any{"s1", nil, []any{}, nil, tree})
	if rc == nil {
		t.Fatal("nil live tree")
	}

	var astral int
	var walk func(s RichSpan)
	walk = func(s RichSpan) {
		if s.Text != "" && len(s.Children) == 0 {
			span := s.End - s.Start
			if span != utf16CodeUnits(s.Text) {
				t.Errorf("leaf [%d,%d) span=%d but utf16Len(%q)=%d",
					s.Start, s.End, span, s.Text, utf16CodeUnits(s.Text))
			}
			if r := len([]rune(s.Text)); span != r {
				astral++ // a leaf where UTF-16 length diverges from rune count
			}
		}
		for _, c := range s.Children {
			walk(c)
		}
	}
	for _, b := range rc.Blocks {
		walk(b)
	}
	if astral == 0 {
		t.Error("fixture no longer exercises the astral (span != rune-len) case")
	}
}

// loadListBlockTree reads the list-block fixture (fabricated text, real
// structure): an intro paragraph followed by two flush bullets and one nested
// bullet, as TOP-LEVEL sibling spans — the shape a real answer's list has.
func loadListBlockTree(t *testing.T) any {
	t.Helper()
	raw, err := os.ReadFile("testdata/list_block_fixture.json")
	if err != nil {
		t.Fatal(err)
	}
	var tree any
	if err := json.Unmarshal(raw, &tree); err != nil {
		t.Fatal(err)
	}
	return tree
}

// TestDecodeListItems checks the ListItem node is captured off content[3]: three
// list-item spans decode with a bullet and their nesting depth read from the
// node's index-[2] element — NOT from the marker's position counters (104), so a
// nested item whose sequence_index is 0 still decodes nesting 1.
func TestDecodeListItems(t *testing.T) {
	tree := loadListBlockTree(t)
	rc := decodeRichContentFromSegment([]any{"s1", nil, []any{}, nil, tree})
	if rc == nil {
		t.Fatal("nil tree from list fixture")
	}
	if len(rc.Blocks) != 4 {
		t.Fatalf("got %d blocks, want 4 (intro + 3 items)", len(rc.Blocks))
	}

	// Block 0 is the intro paragraph: a group, no ListItem.
	if rc.Blocks[0].ListItem != nil {
		t.Errorf("intro paragraph decoded a ListItem: %+v", rc.Blocks[0].ListItem)
	}

	// Blocks 1..3 are list items with bullet "•" and nesting 0, 0, 1.
	wantNesting := []int{0, 0, 1}
	for i, want := range wantNesting {
		b := rc.Blocks[i+1]
		if b.ListItem == nil {
			t.Errorf("block[%d] has no ListItem", i+1)
			continue
		}
		if b.ListItem.Bullet != "•" {
			t.Errorf("block[%d] bullet = %q, want •", i+1, b.ListItem.Bullet)
		}
		if b.ListItem.Nesting != want {
			t.Errorf("block[%d] nesting = %d, want %d", i+1, b.ListItem.Nesting, want)
		}
	}

	// Guard: the nested item (block 3) has sequence_index 0 on the wire, so if
	// nesting were mistakenly read from 104 it would decode as 0. It must be 1.
	if got := rc.Blocks[3].ListItem; got == nil || got.Nesting != 1 || got.Sequence != 0 {
		t.Errorf("nested item = %+v, want Nesting=1 Sequence=0 (nesting must not come from sequence)", got)
	}
}

func looksLikeUUID(s string) bool {
	// 00000000-0000-4000-8000-000000000006 shape: 36 chars with dashes.
	if len(s) != 36 {
		return false
	}
	return s[8] == '-' && s[13] == '-' && s[18] == '-' && s[23] == '-'
}
