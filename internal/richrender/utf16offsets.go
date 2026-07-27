package richrender

import "unicode/utf16"

// Wire offset space.
//
// NotebookLM's answer-body span tree and its citation reply-spans index the
// answer text in UTF-16 code units, not runes: an astral-plane character (an
// emoji, a rare CJK extension) is ONE rune but TWO UTF-16 units, so a span that
// covers it is one unit wider than its rune count. The renderers slice
// []rune(content), so a raw wire offset lands one position early for every
// astral character before it, shifting every downstream [N] marker and
// grounded-span underline.
//
// utf16ToRune converts a wire (UTF-16) offset into content to the rune index at
// that same boundary, so the rune-based renderer stays correct. Building the map
// once per answer keeps each lookup O(1); for the common all-BMP answer the map
// is the identity and the conversion is a no-op.

// utf16RuneMap maps UTF-16 code-unit offsets in a fixed string to rune indices.
type utf16RuneMap struct {
	// runeAt[u] is the rune index at UTF-16 offset u. Length is the string's
	// UTF-16 length + 1, so an end offset (one past the last unit) maps too.
	runeAt []int
}

// newUTF16RuneMap builds the offset map for content. Each rune advances the rune
// index by one and the UTF-16 cursor by one (BMP) or two (astral); both units of
// an astral rune map to that rune's index, so a wire offset landing mid-pair
// resolves to the rune it belongs to rather than past it.
func newUTF16RuneMap(content string) utf16RuneMap {
	runes := []rune(content)
	runeAt := make([]int, 0, len(runes)+1)
	for i, r := range runes {
		runeAt = append(runeAt, i)
		if utf16.RuneLen(r) == 2 {
			runeAt = append(runeAt, i) // second surrogate unit maps to the same rune
		}
	}
	runeAt = append(runeAt, len(runes)) // end offset
	return utf16RuneMap{runeAt: runeAt}
}

// rune returns the rune index for a UTF-16 offset, clamped to the valid range so
// an out-of-bounds wire offset degrades to an edge rather than panicking.
func (m utf16RuneMap) rune(u16 int) int {
	if u16 < 0 {
		return 0
	}
	if u16 >= len(m.runeAt) {
		return m.runeAt[len(m.runeAt)-1]
	}
	return m.runeAt[u16]
}

// utf16Len returns the length of content in UTF-16 code units — the unit the
// wire's answer offsets count in. It equals len([]rune(content)) plus one for
// each astral-plane rune.
func utf16Len(content string) int {
	n := 0
	for _, r := range content {
		n += utf16.RuneLen(r)
	}
	return n
}

// byteToUTF16 maps each byte offset in content (including len(content)) to its
// UTF-16 code-unit offset, so a regexp's byte match positions convert to the
// wire's offset space. A byte offset in the interior of a multibyte rune maps to
// that rune's starting UTF-16 offset. Marker matches always start on a rune
// boundary (the "[" of "[N]"), so only boundary lookups are load-bearing; the
// interior fill just keeps the map total.
func byteToUTF16(content string) map[int]int {
	out := make(map[int]int, len(content)+1)
	u16 := 0
	nextBoundary := 0 // byte offset of the current rune's start
	for bi, r := range content {
		// Any bytes between the previous boundary and this one are interior bytes
		// of the previous rune; map them to that rune's UTF-16 start.
		for b := nextBoundary; b < bi; b++ {
			out[b] = out[nextBoundary]
		}
		out[bi] = u16
		u16 += utf16.RuneLen(r)
		nextBoundary = bi
	}
	for b := nextBoundary; b < len(content); b++ {
		out[b] = out[nextBoundary]
	}
	out[len(content)] = u16
	return out
}
