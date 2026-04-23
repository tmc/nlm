package api

import (
	"encoding/json"
	"fmt"
	"strings"
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
	Start int // inclusive start character offset.
	End   int // exclusive end character offset. Text length == End - Start.
	Text  string
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
	want := l.Fragments[0].Start
	for _, f := range l.Fragments {
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
func (c *Client) LoadSourceText(sourceID, notebookID string) (LoadSourceText, error) {
	raw, err := c.LoadSourceRaw(sourceID, notebookID)
	if err != nil {
		return LoadSourceText{}, err
	}
	return decodeLoadSourceText(raw)
}

// DecodeLoadSourceText decodes a raw hizoJc response into LoadSourceText.
func DecodeLoadSourceText(raw json.RawMessage) (LoadSourceText, error) {
	return decodeLoadSourceText(raw)
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
	for _, fr := range fragments {
		chunks, err := extractChunks(fr)
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
func extractChunks(raw json.RawMessage) ([]TextFragment, error) {
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

	var out []TextFragment
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
		var textArr []json.RawMessage
		if err := json.Unmarshal(triple[2], &textArr); err != nil {
			continue
		}
		if len(textArr) == 0 {
			continue
		}
		var text string
		if err := json.Unmarshal(textArr[0], &text); err != nil {
			// Non-string payload (e.g. PDF image URL triples have a URL,
			// not a plain text string at this position). Skip.
			continue
		}
		out = append(out, TextFragment{Start: start, End: end, Text: text})
	}
	return out, nil
}

func isJSONNull(raw json.RawMessage) bool {
	s := strings.TrimSpace(string(raw))
	return s == "null"
}
