package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// LoadSourceText is the decoded full-text body of a source as returned by
// hizoJc. The server returns the text as a list of (start, end, text)
// triples; the full text is reconstructed by concatenating in order.
//
// Character offsets are server-indexed — the same coordinate space that
// chat-stream Citation events (StartChar/EndChar) reference. That makes this
// the authoritative mapping from citation offset back to quoted text.
type LoadSourceText struct {
	SourceID  string
	Title     string
	Fragments []TextFragment // in ascending start order.
}

// TextFragment is one contiguous piece of the indexed text.
type TextFragment struct {
	Start      int // inclusive start character offset.
	End        int // exclusive end character offset. Text length == End - Start.
	Text       string
	ImageURL   string // transient source image URL, if this is an image fragment.
	ImageID    string // opaque source image ID, if this is an image fragment.
	ListMarker string // list marker decoded from wrapper metadata, if present.
	Bold       bool   // text was marked bold in the chunk attributes.
	Italic     bool   // text was marked italic in the chunk attributes.
	BlockStart bool   // fragment starts an outer hizoJc content block.
}

// IsImage reports whether f names an image payload rather than text.
func (f TextFragment) IsImage() bool {
	return f.ImageURL != ""
}

// Full returns the reconstructed full body text. Gaps between fragments
// (rare but observed when the server splits on section boundaries) are
// padded with a single space, which matches the character count the server
// uses for citation offsets.
func (l LoadSourceText) Full() string {
	if len(l.Fragments) == 0 {
		return ""
	}
	var b strings.Builder
	// Seed the cursor from the first non-image fragment. Before image
	// fragments were decoded they were absent, so the text stream began at the
	// first text fragment with no leading gap; seeding from an image fragment's
	// start would inject spurious leading spaces that never existed.
	want := -1
	for _, f := range l.Fragments {
		if !f.IsImage() {
			want = f.Start
			break
		}
	}
	if want < 0 {
		return ""
	}
	for _, f := range l.Fragments {
		// Preserve the historical text-only representation. Before image
		// fragments were decoded they were absent, so their offset range was
		// rendered as a gap. Do not let an image change Full's byte output.
		if f.IsImage() {
			continue
		}
		for want < f.Start {
			b.WriteByte(' ')
			want++
		}
		b.WriteString(f.Text)
		want = f.End
	}
	return b.String()
}

// Slice returns the text between server offsets [start, end). Returns the
// empty string if the range falls entirely outside the indexed content.
func (l LoadSourceText) Slice(start, end int) string {
	if start >= end {
		return ""
	}
	var b strings.Builder
	cursor := start
	overlap := false
	for _, f := range l.Fragments {
		// See Full: source read's default text view continues to treat image
		// offsets as gaps for compatibility with existing callers.
		if f.IsImage() {
			continue
		}
		if f.End <= start {
			continue
		}
		if f.Start >= end {
			break
		}
		overlap = true
		if cursor < f.Start {
			gapEnd := f.Start
			if gapEnd > end {
				gapEnd = end
			}
			for cursor < gapEnd {
				b.WriteByte(' ')
				cursor++
			}
		}
		lo := start - f.Start
		if lo < 0 {
			lo = 0
		}
		hi := end - f.Start
		chunk := sliceRunes(f.Text, lo, hi)
		b.WriteString(chunk)
		cursor += len([]rune(chunk))
	}
	if !overlap {
		return ""
	}
	for cursor < end {
		b.WriteByte(' ')
		cursor++
	}
	return b.String()
}

func sliceRunes(s string, start, end int) string {
	if start < 0 {
		start = 0
	}
	r := []rune(s)
	if start > len(r) {
		start = len(r)
	}
	if end > len(r) {
		end = len(r)
	}
	if end < start {
		end = start
	}
	return string(r[start:end])
}

// LoadSourceText fetches the text body of a source via hizoJc and decodes
// the positional wire response into a typed LoadSourceText. The PDF variant
// of hizoJc returns per-page image URLs instead of text; on that shape this
// function returns Fragments == nil and no error.
func (c *Client) LoadSourceText(ctx context.Context, sourceID, notebookID string) (LoadSourceText, error) {
	raw, err := c.LoadSourceRaw(ctx, sourceID, notebookID)
	if err != nil {
		return LoadSourceText{}, err
	}
	var response pb.LoadSourceResponse
	if err := c.unmarshal(raw, &response); err == nil {
		return loadSourceTextFromProto(&response), nil
	}
	return decodeLoadSourceText(raw)
}

// DecodeLoadSourceText decodes a raw hizoJc response into LoadSourceText.
func DecodeLoadSourceText(raw json.RawMessage) (LoadSourceText, error) {
	return decodeLoadSourceText(raw)
}

func loadSourceTextFromProto(response *pb.LoadSourceResponse) LoadSourceText {
	var out LoadSourceText
	if response == nil {
		return out
	}
	if source := response.GetSource(); source != nil {
		if id := source.GetSourceId(); id != nil {
			out.SourceID = id.GetSourceId()
		}
		out.Title = source.GetTitle()
	}
	content := response.GetContent()
	if content == nil || content.GetRows() == nil {
		return out
	}
	for i, row := range content.GetRows().GetRows() {
		if row == nil || row.GetText() == nil {
			continue
		}
		// Every row but the first opens an outer content block, and only that
		// row's first fragment carries the flag. Mirrors extractFragments,
		// which passes i != 0 to extractChunks and there ands it with first.
		blockStart := i != 0
		first := true
		marker := listMarkerFromProto(row.GetText().GetListItem())
		for _, span := range row.GetText().GetSpans() {
			if span == nil || span.GetText() == nil {
				continue
			}
			bold, italic := spanStyleFromProto(span.GetText().GetFlags())
			out.Fragments = append(out.Fragments, TextFragment{
				Start:      int(span.GetStart()),
				End:        int(span.GetEnd()),
				Text:       span.GetText().GetText(),
				ListMarker: marker,
				Bold:       bold,
				Italic:     italic,
				BlockStart: blockStart && first,
			})
			first = false
		}
	}
	return out
}

// spanStyleFromProto reproduces decodeTextStyle against the decoded flags.
//
// decodeTextStyle reads the style slot positionally and only reports bold when
// that slot's first element is the scalar true. No capture carries that shape:
// every style slot in the corpus is either empty or a nested array, the
// bold-looking form being ["bar", [true]] rather than ["bar", true]. The bold
// branch is therefore unreachable, and the legacy projection reports bold for
// no span at all — italic alone survives, read from position 2.
//
// The proto models that [true] as flags.bold, so returning the field directly
// would report bold where the legacy projection never does. Whether [true]
// ought to mean bold is a live question — it plausibly should, and 250 spans
// carry it — but answering it would change user-visible source rendering. This
// mirrors the legacy behavior exactly and leaves the semantics to a separate,
// deliberate change.
func spanStyleFromProto(flags *pb.LoadedSourceSpanFlags) (bold, italic bool) {
	return false, flags.GetItalic()
}

// listMarkerFromProto returns the bullet glyph of a list item, or "" when the
// row is not a list item. ListItemMarker is a shape union: most items carry
// the marker object at field 4, while a newer variant puts metadata there and
// shifts the marker to field 5. The legacy decoder finds either by searching
// for wire key 101, which is ListMarker.bullet.
func listMarkerFromProto(item *pb.ListItem) string {
	if item == nil {
		return ""
	}
	if marker := item.GetMarker().GetMarker(); marker != nil {
		return marker.GetBullet()
	}
	return item.GetTrailingMarker().GetBullet()
}

// decodeLoadSourceText parses the positional wire shape observed against a
// 2026-04-22 HAR capture:
//
//	[
//	  [["source_id"], "title", [null, content_len, ...], [null, settings_enum]],
//	  null, null,
//	  [ [ [ [start, end, [[[start, end, ["chunk"]], ...]]], ... ] ] ]
//	]
//
// The top-level length is 4. resp[0] is metadata, resp[3] carries the body.
// Body is doubly wrapped; the inner array is a flat list of fragments.
// Each fragment is [start, end, [[[sub_start, sub_end, ["text"]], ...]]].
// Non-text sources (PDFs, Drive) use the same shape but the text slot
// carries URLs instead — we skip fragments whose string payload doesn't
// look like text.
func decodeLoadSourceText(raw json.RawMessage) (LoadSourceText, error) {
	var resp []json.RawMessage
	if err := json.Unmarshal(raw, &resp); err != nil {
		return LoadSourceText{}, fmt.Errorf("decode load source: %w", err)
	}
	if len(resp) < 4 {
		return LoadSourceText{}, fmt.Errorf("decode load source: expected len>=4, got %d", len(resp))
	}

	var out LoadSourceText
	if err := extractSourceMeta(resp[0], &out); err != nil {
		return LoadSourceText{}, fmt.Errorf("decode meta: %w", err)
	}

	// Body lives at resp[3]; may be null for non-text sources.
	if isJSONNull(resp[3]) {
		return out, nil
	}
	frags, err := extractFragments(resp[3])
	if err != nil {
		return LoadSourceText{}, fmt.Errorf("decode body fragments: %w", err)
	}
	out.Fragments = frags
	return out, nil
}

func extractSourceMeta(raw json.RawMessage, out *LoadSourceText) error {
	var meta []json.RawMessage
	if err := json.Unmarshal(raw, &meta); err != nil {
		return err
	}
	if len(meta) < 2 {
		return nil
	}
	var idList []string
	if err := json.Unmarshal(meta[0], &idList); err == nil && len(idList) > 0 {
		out.SourceID = idList[0]
	}
	var title string
	if err := json.Unmarshal(meta[1], &title); err == nil {
		out.Title = title
	}
	return nil
}

// extractFragments walks the double-wrapped body array and returns the
// flat fragment list. It pulls text from the innermost chunk triples so
// the reconstructed offsets match what the server uses for citations
// regardless of how it grouped fragments at the outer level.
func extractFragments(raw json.RawMessage) ([]TextFragment, error) {
	// body = [[fragments]]
	var l1 []json.RawMessage
	if err := json.Unmarshal(raw, &l1); err != nil {
		return nil, err
	}
	if len(l1) == 0 {
		return nil, nil
	}
	var l2 []json.RawMessage
	if err := json.Unmarshal(l1[0], &l2); err != nil {
		return nil, err
	}
	if len(l2) == 0 {
		return nil, nil
	}
	var fragments []json.RawMessage
	if err := json.Unmarshal(l2[0], &fragments); err != nil {
		return nil, err
	}

	var out []TextFragment
	for i, fr := range fragments {
		chunks, err := extractChunks(fr, i != 0)
		if err != nil {
			return nil, err
		}
		out = append(out, chunks...)
	}
	return out, nil
}

// extractChunks decodes one fragment entry of the form:
//
//	[start, end, [[[sub_start, sub_end, ["text"]], ...], ...extras]]
//
// and returns the innermost text triples as flat TextFragments.
func extractChunks(raw json.RawMessage, blockStart bool) ([]TextFragment, error) {
	var top []json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		return nil, err
	}
	if len(top) < 3 {
		return nil, nil
	}
	// top[2] is [[chunks...], maybe_extra].
	var wrap []json.RawMessage
	if err := json.Unmarshal(top[2], &wrap); err != nil {
		return nil, err
	}
	if len(wrap) == 0 {
		return nil, nil
	}
	// wrap[0] is the actual chunk list.
	var chunks []json.RawMessage
	if err := json.Unmarshal(wrap[0], &chunks); err != nil {
		return nil, nil //nolint:nilerr // non-text source; skip silently.
	}

	marker := decodeListMarker(wrap[1:])
	var out []TextFragment
	first := true
	for _, c := range chunks {
		var triple []json.RawMessage
		if err := json.Unmarshal(c, &triple); err != nil {
			continue
		}
		if len(triple) < 3 {
			continue
		}
		var start, end int
		if err := json.Unmarshal(triple[0], &start); err != nil {
			continue
		}
		if err := json.Unmarshal(triple[1], &end); err != nil {
			continue
		}
		if isJSONNull(triple[2]) {
			image, ok := decodeImageFragment(triple, start, end)
			if ok {
				image.ListMarker = marker
				image.BlockStart = blockStart && first
				out = append(out, image)
				first = false
			}
			continue
		}
		var textArr []json.RawMessage
		if err := json.Unmarshal(triple[2], &textArr); err != nil || len(textArr) == 0 {
			continue
		}
		var text string
		if err := json.Unmarshal(textArr[0], &text); err != nil {
			continue
		}
		bold, italic := decodeTextStyle(textArr[1:])
		out = append(out, TextFragment{
			Start:      start,
			End:        end,
			Text:       text,
			ListMarker: marker,
			Bold:       bold,
			Italic:     italic,
			BlockStart: blockStart && first,
		})
		first = false
	}
	return out, nil
}

// decodeTextStyle decodes the compact style forms observed in hizoJc text
// chunks: [true] for bold and [null, true] for italic.
func decodeTextStyle(attrs []json.RawMessage) (bold, italic bool) {
	if len(attrs) == 0 {
		return false, false
	}
	if json.Unmarshal(attrs[0], &bold) == nil && bold {
		return true, false
	}
	var values []json.RawMessage
	if json.Unmarshal(attrs[0], &values) != nil || len(values) < 2 {
		return false, false
	}
	_ = json.Unmarshal(values[1], &italic)
	return false, italic
}

// decodeListMarker recognizes the list marker stored in a fragment wrapper's
// extra metadata. The remaining wrapper metadata is intentionally left
// opaque: captured traffic does not establish its semantics.
func decodeListMarker(extras []json.RawMessage) string {
	for _, raw := range extras {
		if marker := findListMarker(raw); marker != "" {
			return marker
		}
	}
	return ""
}

func findListMarker(raw json.RawMessage) string {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err == nil {
		var marker string
		if err := json.Unmarshal(object["101"], &marker); err == nil {
			return marker
		}
	}
	var values []json.RawMessage
	if err := json.Unmarshal(raw, &values); err != nil {
		return ""
	}
	for _, value := range values {
		if marker := findListMarker(value); marker != "" {
			return marker
		}
	}
	return ""
}

// decodeImageFragment decodes the inline image shape captured from hizoJc:
// [start, end, null, [url, null, opaque_image_id]]. The URL is transient;
// callers must not assume it is durable or fetch it as part of source read.
func decodeImageFragment(triple []json.RawMessage, start, end int) (TextFragment, bool) {
	if len(triple) < 4 {
		return TextFragment{}, false
	}
	var image []json.RawMessage
	if err := json.Unmarshal(triple[3], &image); err != nil || len(image) < 3 {
		return TextFragment{}, false
	}
	var url, id string
	if err := json.Unmarshal(image[0], &url); err != nil || url == "" {
		return TextFragment{}, false
	}
	if err := json.Unmarshal(image[2], &id); err != nil {
		return TextFragment{}, false
	}
	return TextFragment{Start: start, End: end, ImageURL: url, ImageID: id}, true
}

func isJSONNull(raw json.RawMessage) bool {
	s := strings.TrimSpace(string(raw))
	return s == "null"
}
