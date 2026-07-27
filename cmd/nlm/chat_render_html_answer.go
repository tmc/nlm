package main

import (
	"fmt"
	"html/template"
	"regexp"
	"strconv"
	"strings"
)

// Server-side answer-body rendering.
//
// The answer body — the assistant's prose with its inline [N] citation links
// and grounded-passage underlines — is built HERE, in Go, with html/template's
// contextual auto-escaping, and shipped as pre-rendered HTML in the page. The
// inline script no longer builds the answer DOM; it only attaches interactivity
// (hovercards, cross-highlight, click-to-pin) to the elements this file emits,
// finding them by their data-* attributes.
//
// This is the XSS boundary. Every string derived from server data — the answer
// text, the sliced grounded passage, the literal [N] token text — flows through
// html/template, so it is auto-escaped. The only template.HTML values this file
// produces come from data IT generated (element tags and class names), never
// from server text. A payload like `</script><img onerror=alert(1)>[1]` in the
// answer therefore renders as inert, escaped text.

// answerNode is one piece of a rendered answer: either a run of plain text
// (escaped by the template) or a structural element (tag) wrapping child nodes,
// optionally carrying the data-* attributes the interactivity layer keys on.
//
// A node with Tag == "" is a text leaf: its Text is emitted verbatim through
// the template's HTML-escaping. A node with a Tag is an element; its Text (for a
// leaf element like a text node inside a span) or Children (for a container) are
// rendered inside it. Class/DataMsg/DataCite/Href are attribute values, all
// escaped by the template when set.
type answerNode struct {
	Tag      string // "" → text leaf; else element name (p, ul, li, span, a, hr, h4, div)
	Class    string // class attribute; "" → omit
	Href     string // href attribute (for <a>); "" → omit
	DataMsg  string // data-msg attribute; "" → omit
	DataCite string // data-cite attribute; "" → omit
	Text     string // text content for a leaf/inline node (auto-escaped)
	Children []answerNode
}

// renderAnswerBody renders one assistant turn's answer to escaped HTML. When the
// turn carries a decoded rich tree AND its flat content is newline-free (the
// server strips newlines only from the structured form), the answer is built
// from the block tree so paragraphs, lists, headings, and separators come back.
// Otherwise it is rendered as a single flat block, preserving the current
// behavior — literal [N] markers and any inline Markdown pass through as text so
// nothing regresses.
//
// markers is the same per-marker model the citation cards/rail consume; here it
// supplies each marker's grounded Span and the join key that turns a literal [N]
// into a link. msgIdx keys the data-msg attribute so the script can scope its
// element lookups per turn.
func renderAnswerBody(msgIdx int, m chatDocMessage, markers []htmlMarker) (template.HTML, error) {
	nodes := answerNodes(msgIdx, m, markers)
	var sb strings.Builder
	for _, n := range nodes {
		if err := renderAnswerNode(&sb, n); err != nil {
			return "", err
		}
	}
	return template.HTML(sb.String()), nil
}

// renderAnswerNode writes one node to sb, recursing into children. Each element
// kind has a FIXED-tag template (elemTemplates), so html/template's contextual
// escaping applies correctly: dynamic tag names defeat that analysis, so the tag
// is never data — only the class/href/data-* attribute VALUES and the text are
// interpolated, and every one is auto-escaped in its element/attribute context.
// A node with an empty Tag is a bare text leaf: its Text is HTML-escaped and
// written directly.
func renderAnswerNode(sb *strings.Builder, n answerNode) error {
	if n.Tag == "" {
		sb.WriteString(template.HTMLEscapeString(n.Text))
		return nil
	}
	if n.Tag == "hr" {
		sb.WriteString("<hr>")
		return nil
	}
	t := elemTemplates[n.Tag]
	if t == nil {
		// An unknown tag would be a programming error; fail loudly rather than
		// emit an unescaped or malformed element.
		return fmt.Errorf("answer render: unknown element tag %q", n.Tag)
	}
	// Render this element's open tag + text (leaf) or a children placeholder.
	var inner template.HTML
	if len(n.Children) > 0 {
		var cb strings.Builder
		for _, c := range n.Children {
			if err := renderAnswerNode(&cb, c); err != nil {
				return err
			}
		}
		inner = template.HTML(cb.String())
	}
	return t.Execute(sb, elemData{
		Class:    n.Class,
		Href:     n.Href,
		DataMsg:  n.DataMsg,
		DataCite: n.DataCite,
		Text:     n.Text,
		HasKids:  len(n.Children) > 0,
		Inner:    inner,
	})
}

// elemData is the per-element interpolation set. Class/Href/DataMsg/DataCite and
// Text are server-derived and auto-escaped by the fixed-tag templates; Inner is
// the already-rendered (and already-escaped) child HTML.
type elemData struct {
	Class    string
	Href     string
	DataMsg  string
	DataCite string
	Text     string
	HasKids  bool
	Inner    template.HTML
}

// answerNodes projects a turn into the top-level answer nodes. It chooses the
// tree layout when it applies and otherwise a single flat block.
//
// Wire offsets (the tree's block/run bounds and each marker's grounded Span)
// index the answer in UTF-16 code units; the renderer slices []rune(content).
// The two spaces agree only for all-BMP text, so every wire offset is mapped to
// its rune index here, at the single seam, before the rune-based rendering runs.
// markers is copied (with translated Spans) so the caller's slice is untouched.
func answerNodes(msgIdx int, m chatDocMessage, markers []htmlMarker) []answerNode {
	u16 := newUTF16RuneMap(m.Content)
	markers = translateMarkerSpans(markers, u16)
	byIndex := markersByIndex(markers)
	if shouldReflowFromTree(m.Rich, m.Content) {
		if nodes := treeAnswerNodes(msgIdx, m, markers, byIndex, u16); nodes != nil {
			return nodes
		}
	}
	// Flat fallback: the whole answer as one paragraph, with markers and grounded
	// spans injected across its full rune range.
	runes := []rune(m.Content)
	return []answerNode{{
		Tag:      "div",
		Class:    "answer-block",
		Children: inlineNodes(msgIdx, runes, 0, len(runes), m.Content, markers, byIndex),
	}}
}

// translateMarkerSpans returns a copy of markers whose grounded Spans are mapped
// from UTF-16 wire offsets to rune offsets. Markers without a Span are copied
// unchanged. The input slice and its htmlSpan values are not mutated.
func translateMarkerSpans(markers []htmlMarker, u16 utf16RuneMap) []htmlMarker {
	if len(markers) == 0 {
		return markers
	}
	out := make([]htmlMarker, len(markers))
	copy(out, markers)
	for i := range out {
		if out[i].Span != nil {
			out[i].Span = &htmlSpan{
				Start: u16.rune(out[i].Span.Start),
				End:   u16.rune(out[i].Span.End),
			}
		}
	}
	return out
}

// treeAnswerNodes builds the structured answer from the projected block tree.
// Each block's TEXT is the slice of the (newline-free) content covered by the
// block's [Start,End) rune range — NOT the tree's leaf text, which omits the
// literal [N] markers. Slicing content keeps the markers in place and keeps the
// grounded-span and marker offsets (which index content) exact. It returns nil
// when the projection yields no block, so the caller falls back to flat.
//
// The projected block/run offsets are wire (UTF-16) offsets; u16 maps them to
// the rune space the slicing below uses.
func treeAnswerNodes(msgIdx int, m chatDocMessage, markers []htmlMarker, byIndex map[int]htmlMarker, u16 utf16RuneMap) []answerNode {
	blocks := projectRichDocument(m.Rich)
	if len(blocks) == 0 {
		return nil
	}
	blocks = translateBlockOffsets(blocks, u16)
	runes := []rune(m.Content)
	var out []answerNode
	for _, b := range blocks {
		if node, ok := blockNode(msgIdx, b, runes, m.Content, markers, byIndex); ok {
			out = append(out, node)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// translateBlockOffsets maps every offset in the projected block tree from
// UTF-16 wire offsets to rune offsets: each block's Start/End, and the Start/End
// of every run in its paragraph/table/code runs and its list items. The block
// text is then sliced by these rune offsets, keeping [N] markers and grounded
// spans aligned even when the answer contains astral-plane characters.
func translateBlockOffsets(blocks []richBlockOut, u16 utf16RuneMap) []richBlockOut {
	out := make([]richBlockOut, len(blocks))
	for i, b := range blocks {
		b.Start = u16.rune(b.Start)
		b.End = u16.rune(b.End)
		b.Runs = translateRuns(b.Runs, u16)
		if len(b.Items) > 0 {
			items := make([]richItem, len(b.Items))
			for j, it := range b.Items {
				it.Runs = translateRuns(it.Runs, u16)
				items[j] = it
			}
			b.Items = items
		}
		out[i] = b
	}
	return out
}

// translateRuns maps each run's Start/End from UTF-16 to rune offsets, returning
// a new slice; runs without a real range (End <= Start) are copied unchanged.
func translateRuns(runs []richRun, u16 utf16RuneMap) []richRun {
	if len(runs) == 0 {
		return runs
	}
	out := make([]richRun, len(runs))
	for i, r := range runs {
		if r.End > r.Start {
			r.Start = u16.rune(r.Start)
			r.End = u16.rune(r.End)
		}
		out[i] = r
	}
	return out
}

// blockNode maps one projected block to its element. Block text is the content
// slice [Start,End) so inline markers/spans stay aligned. The mapping:
//   - blockSeparator → <hr> (no text);
//   - blockHidden → skipped (not part of the visible answer);
//   - blockList → <ul> with one <li> per item (nest-1/2/3 class, capped at 3);
//   - blockParagraph → <p>, or <h4> when any run carries the heading hint;
//   - blockTable/blockCodeBlock/blockUnknown → <p> flat-text fallback.
//
// The second return is false when the block contributes nothing (hidden, or an
// empty non-separator block).
func blockNode(msgIdx int, b richBlockOut, runes []rune, content string, markers []htmlMarker, byIndex map[int]htmlMarker) (answerNode, bool) {
	switch b.Kind {
	case blockSeparator:
		return answerNode{Tag: "hr"}, true
	case blockHidden:
		return answerNode{}, false
	case blockList:
		return listNode(msgIdx, b, runes, content, markers, byIndex)
	default:
		start, end := clampRange(b.Start, b.End, len(runes))
		children := inlineNodes(msgIdx, runes, start, end, content, markers, byIndex)
		if len(children) == 0 {
			return answerNode{}, false
		}
		tag := "p"
		if b.Kind == blockParagraph && anyHeadingRun(b.Runs) {
			tag = "h4"
		}
		return answerNode{Tag: tag, Children: children}, true
	}
}

// listNode builds a <ul> from a list block, one <li> per item. Each item's text
// is sliced from content by the item's run offsets so its markers and grounded
// spans align; the item's nesting sets a nest-1/2/3 class (capped at depth 3).
func listNode(msgIdx int, b richBlockOut, runes []rune, content string, markers []htmlMarker, byIndex map[int]htmlMarker) (answerNode, bool) {
	var items []answerNode
	for _, item := range b.Items {
		start, end := itemRange(item, b, len(runes))
		children := inlineNodes(msgIdx, runes, start, end, content, markers, byIndex)
		if len(children) == 0 {
			continue
		}
		items = append(items, answerNode{
			Tag:      "li",
			Class:    nestClass(item.Nesting),
			Children: children,
		})
	}
	if len(items) == 0 {
		return answerNode{}, false
	}
	return answerNode{Tag: "ul", Children: items}, true
}

// itemRange returns the content rune range for a list item, from the item's run
// offsets, clamped to the content length and the enclosing block. An item with
// no offsetful runs falls back to the whole block range so its text still shows.
func itemRange(item richItem, b richBlockOut, runeLen int) (int, int) {
	start, end := runsRange(item.Runs)
	if start < 0 {
		start, end = b.Start, b.End
	}
	return clampRange(start, end, runeLen)
}

// runsRange returns the min Start and max End across runs that carry a real
// range, or (-1,-1) when none do (so the caller can fall back).
func runsRange(runs []richRun) (int, int) {
	start, end := -1, -1
	for _, r := range runs {
		if r.End <= r.Start {
			continue
		}
		if start < 0 || r.Start < start {
			start = r.Start
		}
		if r.End > end {
			end = r.End
		}
	}
	return start, end
}

// nestClass maps a list item's nesting depth to its indent class, capped at 3.
// Depth 0 gets no class (the default flush level).
func nestClass(nesting int) string {
	if nesting <= 0 {
		return ""
	}
	if nesting > 3 {
		nesting = 3
	}
	return "nest-" + strconv.Itoa(nesting)
}

// anyHeadingRun reports whether any run in the block carries the inline-heading
// hint, promoting the paragraph to an <h4>.
func anyHeadingRun(runs []richRun) bool {
	for _, r := range runs {
		if r.Heading {
			return true
		}
	}
	return false
}

// clampRange clamps [start,end) into [0,runeLen], returning a valid (possibly
// empty) range. A negative or inverted input yields an empty range at 0.
func clampRange(start, end, runeLen int) (int, int) {
	if start < 0 {
		start = 0
	}
	if end > runeLen {
		end = runeLen
	}
	if start > runeLen {
		start = runeLen
	}
	if end < start {
		end = start
	}
	return start, end
}

// inlineNodes renders the content rune slice [start,end) into inline answer
// nodes, injecting the [N] marker links and grounded-span underlines by offset
// exactly as the client script did — but the text now flows through the template
// escaper. Grounded spans (a marker's validated reply-span) become underlined
// <span class="grounded">; literal [N] tokens become citelink anchors; the text
// between segments is emitted verbatim. Offsets are RUNE offsets into content.
func inlineNodes(msgIdx int, runes []rune, start, end int, content string, markers []htmlMarker, byIndex map[int]htmlMarker) []answerNode {
	if end <= start {
		return nil
	}
	segs := answerSegments(runes, start, end, content, markers)

	var out []answerNode
	cur := start
	for _, s := range segs {
		if s.start < cur { // defensive: skip anything that would overlap
			continue
		}
		if s.start > cur {
			out = append(out, textNode(runes, cur, s.start))
		}
		if s.grounded != nil {
			out = append(out, groundedNode(msgIdx, runes, s.start, s.end, s.grounded.Index))
		} else {
			out = append(out, markerNodes(msgIdx, s.inner, byIndex)...)
		}
		cur = s.end
	}
	if cur < end {
		out = append(out, textNode(runes, cur, end))
	}
	return out
}

// answerSeg is a placed segment inside a block: either a grounded span (grounded
// non-nil) or a literal [N] marker token (inner is the bracket body). start/end
// are rune offsets into content.
type answerSeg struct {
	start, end int
	grounded   *htmlMarker // set → underline this range
	inner      string      // set → the [N] token's inner text, e.g. "1" or "1-4"
}

// answerSegments collects the grounded-span and [N]-marker segments that fall
// within [start,end), sorted by start. A grounded span is placed only when it is
// valid (end>start, inside the block range) — the same shape the Go payload
// already validated globally, re-checked against this block's bounds. Marker
// tokens are found by scanning the content slice with htmlMarkerRe and mapping
// byte offsets back to rune offsets. Grounded spans and marker tokens never
// overlap (the payload validation rejects a span that covers a marker), so the
// merged list needs no conflict resolution beyond the defensive skip in the
// caller.
func answerSegments(runes []rune, start, end int, content string, markers []htmlMarker) []answerSeg {
	var segs []answerSeg
	for i := range markers {
		sp := markers[i].Span
		if sp == nil || sp.End <= sp.Start {
			continue
		}
		if sp.Start < start || sp.End > end {
			continue
		}
		segs = append(segs, answerSeg{start: sp.Start, end: sp.End, grounded: &markers[i]})
	}
	for _, tok := range markerTokens(runes, start, end) {
		segs = append(segs, tok)
	}
	sortSegs(segs)
	return segs
}

// markerTokens finds every [N] marker token whose rune range lies within
// [start,end), returning them as segments carrying the bracket body (inner). It
// scans the content slice of that range and converts the regex's byte offsets to
// rune offsets local to content.
func markerTokens(runes []rune, start, end int) []answerSeg {
	slice := string(runes[start:end])
	var out []answerSeg
	for _, loc := range htmlMarkerRe.FindAllStringSubmatchIndex(slice, -1) {
		startRune := start + len([]rune(slice[:loc[0]]))
		endRune := start + len([]rune(slice[:loc[1]]))
		inner := slice[loc[2]:loc[3]]
		out = append(out, answerSeg{start: startRune, end: endRune, inner: inner})
	}
	return out
}

// sortSegs orders segments by start, then end, stably enough for rendering.
func sortSegs(segs []answerSeg) {
	for i := 1; i < len(segs); i++ {
		for j := i; j > 0; j-- {
			a, b := segs[j-1], segs[j]
			if a.start < b.start || (a.start == b.start && a.end <= b.end) {
				break
			}
			segs[j-1], segs[j] = segs[j], segs[j-1]
		}
	}
}

// textNode wraps a content rune slice as a plain text node (auto-escaped by the
// template on render).
func textNode(runes []rune, start, end int) answerNode {
	return answerNode{Text: string(runes[start:end])}
}

// groundedNode builds the underlined grounded-passage span for a marker. Its
// text (the sliced passage) is auto-escaped; the data-msg/data-cite attributes
// are the interactivity layer's join key back to the marker's card and rail.
func groundedNode(msgIdx int, runes []rune, start, end, index int) answerNode {
	return answerNode{
		Tag:      "span",
		Class:    "grounded",
		DataMsg:  strconv.Itoa(msgIdx),
		DataCite: strconv.Itoa(index),
		Text:     string(runes[start:end]),
	}
}

// markerNodes turns one bracketed token body — "12", "1-4", "1, 2, 3" — into
// the "[" … "]" text with a citelink anchor for each contained index we have a
// citation for. This mirrors the client's appendMarkerLinks/appendIndexLink: the
// brackets and separators are kept as literal text, a range expands to one link
// per bound, and an index with no citation stays plain text. Every text piece is
// a text node (escaped); the anchor text is likewise escaped.
func markerNodes(msgIdx int, inner string, byIndex map[int]htmlMarker) []answerNode {
	var out []answerNode
	out = append(out, answerNode{Text: "["})
	for pi, part := range strings.Split(inner, ",") {
		if pi > 0 {
			out = append(out, answerNode{Text: ","})
		}
		lead := part[:len(part)-len(strings.TrimLeft(part, " \t"))]
		if lead != "" {
			out = append(out, answerNode{Text: lead})
		}
		body := strings.TrimSpace(part)
		if lo, sep, hi, ok := splitRange(body); ok {
			out = append(out, indexNode(msgIdx, lo, "", byIndex))
			if sep != "" {
				out = append(out, answerNode{Text: sep})
			}
			out = append(out, indexNode(msgIdx, hi, hi, byIndex))
			continue
		}
		if n, err := strconv.Atoi(body); err == nil {
			out = append(out, indexNode(msgIdx, strconv.Itoa(n), "", byIndex))
			continue
		}
		if body != "" {
			out = append(out, answerNode{Text: body})
		}
	}
	out = append(out, answerNode{Text: "]"})
	return out
}

// rangeRe matches a trimmed "lo-hi" index range, capturing the low bound, the
// literal separator (the dash and any spaces around it), and the high bound. The
// separator is kept verbatim so "1 - 4" round-trips as written.
var rangeRe = regexp.MustCompile(`^(\d+)(\s*-\s*)(\d+)$`)

// splitRange parses a trimmed "lo-hi" range body, returning the low bound, the
// literal separator between the numbers, the high bound, and ok.
func splitRange(body string) (lo, sep, hi string, ok bool) {
	m := rangeRe.FindStringSubmatch(body)
	if m == nil {
		return "", "", "", false
	}
	return m[1], m[2], m[3], true
}

// indexNode builds one [N] index: a citelink anchor when we have that citation,
// else plain text. shown overrides the displayed label (used for a range's upper
// bound so "1-4" shows "4"); when empty, the index string itself is shown.
func indexNode(msgIdx int, idx, shown string, byIndex map[int]htmlMarker) answerNode {
	if shown == "" {
		shown = idx
	}
	n, err := strconv.Atoi(idx)
	if err != nil {
		return answerNode{Text: shown}
	}
	if _, ok := byIndex[n]; !ok {
		return answerNode{Text: shown}
	}
	return answerNode{
		Tag:      "a",
		Class:    "citelink",
		Href:     "#cite-" + strconv.Itoa(msgIdx) + "-" + strconv.Itoa(n),
		DataMsg:  strconv.Itoa(msgIdx),
		DataCite: strconv.Itoa(n),
		Text:     shown,
	}
}

// markersByIndex indexes the markers by their citation index for O(1) link
// lookup, matching the client's `known` map.
func markersByIndex(markers []htmlMarker) map[int]htmlMarker {
	byIndex := make(map[int]htmlMarker, len(markers))
	for _, m := range markers {
		byIndex[m.Index] = m
	}
	return byIndex
}

// elemTemplates holds one fixed-tag template per element kind the answer body
// can emit. Fixing the tag (rather than interpolating {{.Tag}}) is what lets
// html/template's contextual auto-escaping work: it must know the element to
// choose the right escaper for each attribute and the text body. Every
// interpolated value — Class, Href, DataMsg, DataCite, Text — is server-derived
// and thus auto-escaped in place; Inner is pre-rendered, already-escaped child
// HTML. This map is the closed vocabulary of tags the renderer may produce.
var elemTemplates = func() map[string]*template.Template {
	m := map[string]string{
		"p":    elemBlockSource,
		"h4":   elemBlockSource,
		"ul":   elemBlockSource,
		"li":   elemBlockSource,
		"div":  elemBlockSource,
		"span": elemBlockSource,
		"a":    elemBlockSource,
	}
	out := make(map[string]*template.Template, len(m))
	for tag, src := range m {
		// The template's name IS the tag, so the fixed literal tag comes from the
		// map key, never from data.
		out[tag] = template.Must(template.New(tag).Parse(
			strings.ReplaceAll(src, "TAG", tag)))
	}
	return out
}()

// elemBlockSource is the shared element body. "TAG" is replaced with the fixed
// element name before parsing, so the open/close tags are compile-time literals
// (the tag is data to the string replace, not to html/template). Attributes are
// emitted only when set; the body is either pre-rendered child HTML or escaped
// leaf text.
const elemBlockSource = `<TAG` +
	`{{if .Class}} class="{{.Class}}"{{end}}` +
	`{{if .Href}} href="{{.Href}}"{{end}}` +
	`{{if .DataMsg}} data-msg="{{.DataMsg}}"{{end}}` +
	`{{if .DataCite}} data-cite="{{.DataCite}}"{{end}}>` +
	`{{if .HasKids}}{{.Inner}}{{else}}{{.Text}}{{end}}` +
	`</TAG>`
