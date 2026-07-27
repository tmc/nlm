package api

// Rich answer-body content.
//
// A GetConversationHistory assistant turn carries, alongside the flat answer
// text, a span tree describing the answer's structure — paragraph breaks,
// lists, separators, and inline emphasis. On the wire the tree lives at
// arr[4][0][4]: a list of blocks, each a span
//
//	[start, end, content]
//
// where content is either a leaf ([text] or [text, marks]) or a group of
// nested child spans optionally followed by a block-marks trailer [null, N].
// Offsets are UTF-16 code-unit offsets into the flat answer (an astral rune
// counts as two), so a block and a
// citation reply_span index the same coordinate space.
//
// The whole answer ships with its newlines stripped, so this tree is the only
// place the paragraph/list structure survives; a flat renderer cannot rebuild
// it. RichContent is the decoded tree; the cmd-side renderer projects it to
// HTML/Markdown/text. Decoding is additive: parseConversationHistory still
// fills ChatMessage.Content with the flat text as the floor, and Rich is nil
// when a turn carries no tree (or it fails to decode), so every renderer keeps
// working on Content alone.

// RichContent is a decoded answer-body span tree: the ordered block spans over
// the flat answer text. A nil *RichContent means "no tree" — render flat
// Content.
type RichContent struct {
	Blocks []RichSpan
}

// RichSpan is one span of the answer. Spans nest: a block's
// content is a group of child spans; a leaf's content is text. Start/End are
// UTF-16 code-unit offsets into the flat answer (End exclusive; an astral rune
// counts as two units). The structural type is
// which content field is populated — a Leaf, a Group of children, or a bare
// Separator boundary — mirroring the wire, not a label. Marks on the group are
// the block-level trailer ([null, N]); marks on a leaf are its inline TextMarks.
//
// A group span whose content carries a ListItem trailer is a list item;
// ListItem holds its bullet, kind, and nesting. On the wire the items are
// TOP-LEVEL sibling spans (interleaved with paragraphs), one per bullet — not
// children of a single list group — so the renderer coalesces consecutive
// list-item spans into one list.
type RichSpan struct {
	Start    int
	End      int
	Text     string        // leaf text; "" for a group or separator
	Children []RichSpan    // group content; nil for a leaf
	Marks    []bool        // inline TextMarks flags (leaf) — positional, may be short
	BlockTag int           // block-level trailer tag ([null, N]); 0 when absent
	HasTag   bool          // a [null, N] trailer was present (distinguishes N==0)
	ListItem *RichListItem // set → this group span is a list item
}

// RichListItem is the list-item marker on a group span. Its presence marks the
// span as a list item; consecutive list-item spans form one list. On the wire it
// is a trailing element of the group content (at content index 3), shaped
//
//	[null, null, NESTING, {101: bullet, 102: kind, 103: number, 104: sequence}]
//
// where NESTING is the node's index-[2] element and the trailing object's string
// keys are ListMarker proto fields: 101 bullet glyph, 102 marker_kind (1 =
// unordered/bullet; 2 = ordered/numbers, unverified), 103 item_number (1-based,
// resets per list), 104 sequence_index (0-based, resets per list — a 0 marks the
// start of a new list run).
type RichListItem struct {
	Bullet   string // 101: bullet glyph, e.g. "•"; "" when absent
	Kind     int    // 102: marker_kind — 1 unordered, 2 ordered (ordered unverified)
	Nesting  int    // node[2]: nesting depth, 0 at the flush (outermost) level
	Sequence int    // 104: 0-based index within the list; 0 starts a new list run
}

// decodeRichContent decodes the answer-body span tree from a GetConversationHistory
// message's content block (arr[4]). It returns nil when the block carries no
// recognizable tree, so the caller falls back to flat text — decoding never
// fails a message.
//
// The tree is at block[0][4]: block[0] is the primary content segment
// ["s1", null, [ids], null, TREE]. decodeRichContent unwraps the outer block to
// its first segment, then defers to decodeRichContentFromSegment.
func decodeRichContent(contentBlock any) *RichContent {
	block, ok := contentBlock.([]any)
	if !ok || len(block) == 0 {
		return nil
	}
	return decodeRichContentFromSegment(block[0])
}

// decodeRichContentFromSegment decodes the span tree from a content SEGMENT —
// ["s1", null, [ids], null, TREE]. This is the shape that arrives directly on
// the live GenerateFreeFormStreamed path (the frame's inner[0]); the replay
// path reaches the same segment after unwrapping the outer block (block[0]).
// Both share this one decoder, so the grammar is defined in exactly one place.
//
// The tree is at segment[4]. TREE is a LAYERED, wrapped array — the block list
// is not TREE itself but sits under a couple of wrapper layers, alongside null
// layers and a grounding/source-attribution layer. Rather than hard-code the
// depth, findBlockList descends the leading element until it reaches the layer
// whose elements are spans ([start, end, ...]). Grounding is left to
// parseCitationsV2 (from arr[1]/arr[2] live, arr[4][3] replay); this decoder
// only walks span nodes, so a grounding sub-layer — shaped [[uuid], [null, s,
// e]] — is never mistaken for a block (its first element is not a numeric
// offset). Returns nil when the segment carries no recognizable tree.
func decodeRichContentFromSegment(seg any) *RichContent {
	segment, ok := seg.([]any)
	if !ok || len(segment) < 5 {
		return nil
	}
	tree, ok := segment[4].([]any)
	if !ok || len(tree) == 0 {
		return nil
	}

	blockList := findBlockList(tree)
	if blockList == nil {
		return nil
	}

	var blocks []RichSpan
	for _, raw := range blockList {
		if s, ok := decodeSpan(raw); ok {
			blocks = append(blocks, s)
		}
	}
	if len(blocks) == 0 {
		return nil
	}
	return &RichContent{Blocks: blocks}
}

// findBlockList descends the leading element of the wrapped tree until it finds
// the layer that IS a block list — a list whose first element is a span
// ([start, end, ...] with numeric offsets). It returns that layer, or nil when
// no span layer is reachable (a tree-less or unrecognized frame). The bounded
// descent tolerates the wire's extra wrapper layers without hard-coding a depth.
func findBlockList(node any) []any {
	for depth := 0; depth < 8; depth++ {
		list, ok := node.([]any)
		if !ok || len(list) == 0 {
			return nil
		}
		if looksLikeBlockList(list) {
			return list
		}
		node = list[0] // descend the leading wrapper layer
	}
	return nil
}

// looksLikeBlockList reports whether v is a list whose first element is itself a
// span ([start, end, ...] with numeric offsets) — the signal that v is the
// block-list layer rather than a wrapper or a non-block (grounding) layer.
func looksLikeBlockList(v []any) bool {
	if len(v) == 0 {
		return false
	}
	first, ok := v[0].([]any)
	if !ok || len(first) < 2 {
		return false
	}
	_, sOK := first[0].(float64)
	_, eOK := first[1].(float64)
	return sOK && eOK
}

// decodeSpan decodes one span node [start, end, content, ...]. content is a
// leaf ([text] or [text, marks]) or a group of child spans optionally followed
// by a block trailer [null, N]. A node with no numeric start/end (e.g. a null
// grounding-layer placeholder) is skipped (ok=false).
func decodeSpan(raw any) (RichSpan, bool) {
	arr, ok := raw.([]any)
	if !ok || len(arr) < 2 {
		return RichSpan{}, false
	}
	start, sOK := arr[0].(float64)
	end, eOK := arr[1].(float64)
	if !sOK || !eOK {
		return RichSpan{}, false
	}
	span := RichSpan{Start: int(start), End: int(end)}

	// A separator block is [start, end, null, null, ...] with no content group:
	// no third element or an explicit null there. Leave it as a bare span.
	if len(arr) < 3 || arr[2] == nil {
		return span, true
	}

	content, ok := arr[2].([]any)
	if !ok {
		return span, true
	}
	decodeContent(&span, content)
	return span, true
}

// decodeContent fills a span from its content array. Two shapes:
//   - group: [[child…], [null, N]?, null?, listItem?] — the first element is the
//     child span list, an optional second is the block-marks trailer, and a
//     trailing element (index 3) is the ListItem node on a list item;
//   - leaf:  [text] or [text, marks] — the first element is the text string.
func decodeContent(span *RichSpan, content []any) {
	if len(content) == 0 {
		return
	}
	// Leaf: content[0] is the text.
	if text, ok := content[0].(string); ok {
		span.Text = text
		if len(content) > 1 {
			span.Marks = decodeMarks(content[1])
		}
		return
	}
	// Group: content[0] is the child span list.
	if children, ok := content[0].([]any); ok {
		for _, raw := range children {
			if c, ok := decodeSpan(raw); ok {
				span.Children = append(span.Children, c)
			}
		}
	}
	// Optional block trailer [null, N]: the second element carries a block tag.
	if len(content) > 1 {
		if trailer, ok := content[1].([]any); ok && len(trailer) >= 2 {
			if n, ok := trailer[1].(float64); ok {
				span.BlockTag = int(n)
				span.HasTag = true
			}
		}
	}
	// Optional ListItem node (list items only): content[3] is
	// [null, null, NESTING, {101: bullet, 102: kind, 103: number, 104: sequence}].
	if len(content) > 3 {
		span.ListItem = decodeListItem(content[3])
	}
}

// decodeListItem decodes the ListItem node of a list item, or nil when node is
// not a list-item node. The node is [null, null, NESTING, MARKER]: NESTING at
// index 2 is the nesting depth (NOT any of the marker's position counters), and
// MARKER is an object-encoded map keyed by the ListMarker proto field numbers as
// strings ("101" bullet, "102" kind, "103" number, "104" sequence). A node
// missing the marker object is not a list item.
func decodeListItem(node any) *RichListItem {
	arr, ok := node.([]any)
	if !ok || len(arr) < 4 {
		return nil
	}
	marker, ok := arr[3].(map[string]any)
	if !ok {
		return nil
	}
	item := &RichListItem{}
	if n, ok := arr[2].(float64); ok {
		item.Nesting = int(n)
	}
	if b, ok := marker["101"].(string); ok {
		item.Bullet = b
	}
	if k, ok := marker["102"].(float64); ok {
		item.Kind = int(k)
	}
	if s, ok := marker["104"].(float64); ok {
		item.Sequence = int(s)
	}
	return item
}

// decodeMarks decodes an inline TextMarks flag array to a positional bool slice.
// The wire array is positional (nulls and bools), e.g. [null,...,true]; a true
// at a position sets that flag. Non-bool positions become false. The slice may
// be shorter than a full mark set — callers index defensively.
func decodeMarks(v any) []bool {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	marks := make([]bool, len(arr))
	for i, e := range arr {
		b, _ := e.(bool)
		marks[i] = b
	}
	return marks
}
