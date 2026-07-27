package api

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// A minimal hand-built hizoJc response that mirrors the HAR-verified shape:
// [[source_id], title, metadata-tuple, settings-tuple], null, null, body
// body = [[ [[start,end,chunks], ...] ]]
// chunks = [[[cs,ce,["text"]], ...]]
const sampleHizojcResponse = `[
  [["src-aaa"], "example", [null, 42], [null, 2]],
  null,
  null,
  [[[
    [0, 11, [[[0, 5, ["hello"]], [5, 6, [" "]], [6, 11, ["world"]]]]],
    [11, 17, [[[11, 12, ["\n"]], [12, 17, ["again"]]]]]
  ]]]
]`

func TestDecodeLoadSourceText(t *testing.T) {
	got, err := decodeLoadSourceText(json.RawMessage(sampleHizojcResponse))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.SourceID != "src-aaa" {
		t.Errorf("source_id = %q, want src-aaa", got.SourceID)
	}
	if got.Title != "example" {
		t.Errorf("title = %q, want example", got.Title)
	}
	if len(got.Fragments) != 5 {
		t.Fatalf("fragments = %d, want 5: %+v", len(got.Fragments), got.Fragments)
	}
	if got.Fragments[0].Text != "hello" || got.Fragments[0].Start != 0 || got.Fragments[0].End != 5 {
		t.Errorf("fragments[0] = %+v", got.Fragments[0])
	}
	if got.Fragments[4].Text != "again" {
		t.Errorf("fragments[4] = %+v", got.Fragments[4])
	}
	if got.Fragments[0].BlockStart || !got.Fragments[3].BlockStart || got.Fragments[4].BlockStart {
		t.Errorf("block starts = %#v", got.Fragments)
	}
}

func TestLoadSourceText_Full(t *testing.T) {
	got, err := decodeLoadSourceText(json.RawMessage(sampleHizojcResponse))
	if err != nil {
		t.Fatal(err)
	}
	full := got.Full()
	want := "hello world\nagain"
	if full != want {
		t.Errorf("Full = %q, want %q", full, want)
	}
}

func TestLoadSourceText_Slice(t *testing.T) {
	got, err := decodeLoadSourceText(json.RawMessage(sampleHizojcResponse))
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		start, end int
		want       string
	}{
		{0, 5, "hello"},
		{6, 11, "world"},
		{0, 11, "hello world"},
		{12, 17, "again"},
		{5, 12, " world\n"},
	}
	for _, c := range cases {
		if g := got.Slice(c.start, c.end); g != c.want {
			t.Errorf("Slice(%d,%d) = %q, want %q", c.start, c.end, g, c.want)
		}
	}
}

func TestLoadSourceText_NonTextSource(t *testing.T) {
	// PDF-shape: body fragment payload is a URL instead of a text string.
	// Decoder should return zero fragments without error so callers can
	// fall through to other resolution paths.
	pdfLike := `[[["p"], "pdf.pdf", [null, 99], [null, 2]], null, null, [[[
	  [0, 1, [[[0, 1, null, ["https://example.googleusercontent.com/img"]]]]]
	]]]]`
	got, err := decodeLoadSourceText(json.RawMessage(pdfLike))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Fragments) != 0 {
		t.Errorf("fragments = %d, want 0 for non-text source", len(got.Fragments))
	}
	if got.Title != "pdf.pdf" {
		t.Errorf("title = %q", got.Title)
	}
}

// TestDecodeLoadSourceText_ImageFragment locks the inline image tuple found
// in the captured hizoJc response at
// /tmp/nlm-traffic/.../batchexecute/_query/018.json. Other image nodes in
// that source and in _query/017.json use the same tuple shape.
func TestDecodeLoadSourceText_ImageFragment(t *testing.T) {
	raw, err := os.ReadFile("testdata/load_source_text_image_fragment.json")
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeLoadSourceText(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Fragments) != 3 {
		t.Fatalf("fragments = %d, want 3: %+v", len(got.Fragments), got.Fragments)
	}
	image := got.Fragments[1]
	if image.Start != 7078 || image.End != 7079 {
		t.Errorf("image offsets = [%d,%d), want [7078,7079)", image.Start, image.End)
	}
	if image.ImageURL != "https://lh3.googleusercontent.com/notebooklm/AKYWMX_PFTWg5K888vecw9YHdzxmksiR_RH7pjR4eCbsp7E5Mihlij63i536lVvN8AW4AgMxtQZZQDqCD6tGAMiZWIdbEeY2ppauxzpvaD4bgViNc4KQEKTXQmWRvcTh_ET7mHQIk8A4Kw=w868-h1280-v0" {
		t.Errorf("image URL = %q", image.ImageURL)
	}
	if image.ImageID != "8ea2e3ac-4893-4d17-a831-5264e8e44fa1" {
		t.Errorf("image ID = %q", image.ImageID)
	}
	if got, want := got.Full(), "2 Figure 1: The Transformer - model architecture. "; got != want {
		t.Errorf("Full = %q, want %q", got, want)
	}
}

func TestDecodeLoadSourceText_ListMarker(t *testing.T) {
	raw := json.RawMessage(`[
  [["source"], "title", [null, 1], [null, 2]], null, null,
  [[[
    [0, 4, [[[0, 4, ["item"]]], null, null, [null, null, 0, {"101":"•","102":1}]]]
  ]]]
]`)
	got, err := decodeLoadSourceText(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Fragments) != 1 {
		t.Fatalf("fragments = %d, want 1", len(got.Fragments))
	}
	if got, want := got.Fragments[0].ListMarker, "•"; got != want {
		t.Errorf("list marker = %q, want %q", got, want)
	}
}

func TestDecodeLoadSourceText_TextStyle(t *testing.T) {
	raw := json.RawMessage(`[
  [["source"], "title", [null, 1], [null, 2]], null, null,
  [[[
    [0, 4, [[[0, 2, ["bold", true]], [2, 4, ["italic", [null, true]]]]]]
  ]]]
]`)
	got, err := decodeLoadSourceText(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Fragments) != 2 {
		t.Fatalf("fragments = %d, want 2", len(got.Fragments))
	}
	if !got.Fragments[0].Bold || got.Fragments[0].Italic {
		t.Errorf("bold fragment = %+v", got.Fragments[0])
	}
	if got.Fragments[1].Bold || !got.Fragments[1].Italic {
		t.Errorf("italic fragment = %+v", got.Fragments[1])
	}
}

func TestLoadSourceText_NullBody(t *testing.T) {
	// Minimal valid response with null body; decoder must not crash.
	r := `[[["s"], "t", [null, 0], [null, 2]], null, null, null]`
	got, err := decodeLoadSourceText(json.RawMessage(r))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.SourceID != "s" || got.Title != "t" || len(got.Fragments) != 0 {
		t.Errorf("unexpected: %+v", got)
	}
}

// Sanity: Slice on empty LoadSourceText returns empty.
func TestSlice_Empty(t *testing.T) {
	var l LoadSourceText
	if got := l.Slice(0, 10); got != "" {
		t.Errorf("Slice on empty = %q", got)
	}
	if got := l.Full(); got != "" {
		t.Errorf("Full on empty = %q", got)
	}
}

// Sanity: gap-padding doesn't over-count when fragments are contiguous.
func TestFull_NoGap(t *testing.T) {
	l := LoadSourceText{Fragments: []TextFragment{
		{Start: 0, End: 3, Text: "abc"},
		{Start: 3, End: 6, Text: "def"},
	}}
	if got := l.Full(); got != "abcdef" {
		t.Errorf("Full = %q", got)
	}
	// Full length equals last-fragment End.
	if len(l.Full()) != l.Fragments[len(l.Fragments)-1].End {
		t.Errorf("length mismatch")
	}
}

// Regression: an image as the FIRST fragment must not inject a leading gap.
// Full seeds its cursor from the first non-image fragment; seeding from the
// image's Start would emit spurious leading spaces up to the first text.
func TestFull_LeadingImageNoGap(t *testing.T) {
	l := LoadSourceText{Fragments: []TextFragment{
		{Start: 0, End: 10, ImageURL: "https://example/i.png"},
		{Start: 10, End: 15, Text: "hello"},
	}}
	if got := l.Full(); got != "hello" {
		t.Errorf("Full = %q, want %q", got, "hello")
	}
}

// An image BETWEEN text fragments is still treated as a gap (historical behavior).
func TestFull_InteriorImageIsGap(t *testing.T) {
	l := LoadSourceText{Fragments: []TextFragment{
		{Start: 0, End: 2, Text: "ab"},
		{Start: 2, End: 4, ImageURL: "https://example/i.png"},
		{Start: 4, End: 6, Text: "cd"},
	}}
	if got := l.Full(); got != "ab  cd" {
		t.Errorf("Full = %q, want %q", got, "ab  cd")
	}
}

// Sanity: Slice across fragment boundary concatenates cleanly.
func TestSlice_CrossFragment(t *testing.T) {
	l := LoadSourceText{Fragments: []TextFragment{
		{Start: 0, End: 5, Text: "hello"},
		{Start: 5, End: 11, Text: " world"},
	}}
	if !strings.HasPrefix(l.Slice(3, 8), "lo ") {
		t.Errorf("Slice = %q", l.Slice(3, 8))
	}
}

func TestSlice_PreservesGapPadding(t *testing.T) {
	l := LoadSourceText{Fragments: []TextFragment{
		{Start: 0, End: 1, Text: "a"},
		{Start: 3, End: 4, Text: "b"},
	}}
	if got := l.Slice(0, 4); got != "a  b" {
		t.Fatalf("Slice(0,4) = %q, want %q", got, "a  b")
	}
}

func TestSlice_OutsideContent(t *testing.T) {
	l := LoadSourceText{Fragments: []TextFragment{
		{Start: 3, End: 4, Text: "b"},
	}}
	if got := l.Slice(0, 2); got != "" {
		t.Fatalf("Slice(0,2) = %q, want empty", got)
	}
	if got := l.Slice(4, 6); got != "" {
		t.Fatalf("Slice(4,6) = %q, want empty", got)
	}
}

func TestSlice_UnicodeOffsets(t *testing.T) {
	l := LoadSourceText{Fragments: []TextFragment{
		{Start: 0, End: 3, Text: "a界b"},
	}}
	if got := l.Slice(1, 2); got != "界" {
		t.Fatalf("Slice(1,2) = %q, want %q", got, "界")
	}
	if got := l.Slice(0, 3); got != "a界b" {
		t.Fatalf("Slice(0,3) = %q, want %q", got, "a界b")
	}
}

// Regression: parse a real captured hizoJc response (vz-macos txtar note).
// Runs only when /tmp/hizojc-txtar.json is present — it's the ad-hoc path
// used by `nlm dump-load-source` when probing a live notebook, and skipping
// keeps the test hermetic on CI and fresh checkouts.
func TestDecodeLoadSourceText_Fixture(t *testing.T) {
	path := "/tmp/hizojc-txtar.json"
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("%s not present; skip live fixture", path)
	}
	got, err := decodeLoadSourceText(raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Fragments) == 0 {
		t.Fatalf("no text fragments decoded from fixture")
	}
	// Must be monotonic by Start.
	for i := 1; i < len(got.Fragments); i++ {
		if got.Fragments[i].Start < got.Fragments[i-1].Start {
			t.Errorf("fragments not sorted at %d: %+v vs %+v", i, got.Fragments[i-1], got.Fragments[i])
			break
		}
	}
	t.Logf("decoded %d text fragments from fixture; last offset = %d",
		len(got.Fragments), got.Fragments[len(got.Fragments)-1].End)
}
