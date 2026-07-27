package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf16"

	"github.com/tmc/nlm/internal/notebooklm/api"
)

// renderChatHTMLToDestination renders doc to a self-contained HTML page.
// An explicit "-" writes to stdout. Otherwise the page is written to OutFile,
// or to the render cache when OutFile is empty.
func renderChatHTMLToDestination(doc chatDocument, ctx chatRenderContext, opts chatRenderOptions) error {
	path, err := chatHTMLDestination(doc.NotebookID, doc.ConversationID, opts.OutFile)
	if err != nil {
		return err
	}
	if path == "" {
		return renderChatHTML(os.Stdout, doc, ctx)
	}

	var buf bytes.Buffer
	if err := renderChatHTML(&buf, doc, ctx); err != nil {
		return err
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("write html: %w", err)
	}
	fmt.Fprintf(os.Stderr, "nlm: wrote %s\n", path)

	if opts.Open {
		if err := openInBrowser(path); err != nil {
			fmt.Fprintf(os.Stderr, "nlm: could not open browser: %v\n", err)
		}
	}
	return nil
}

// chatHTMLDestination returns the output path for an HTML conversation render.
// An empty path means stdout.
func chatHTMLDestination(notebookID, conversationID, outFile string) (string, error) {
	if outFile == "-" {
		return "", nil
	}
	if outFile != "" {
		return outFile, nil
	}
	dir, err := renderCacheDir()
	if err != nil {
		return "", fmt.Errorf("create render cache: %w", err)
	}
	dir = filepath.Join(dir, notebookID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create render directory: %w", err)
	}
	return filepath.Join(dir, conversationID+".html"), nil
}

// openInBrowser opens path with the platform's default handler. A failure is
// the caller's to surface as a warning; opening is a convenience, never a
// prerequisite of a successful render.
func openInBrowser(path string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", path)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", path)
	default: // linux and other unix
		cmd = exec.Command("xdg-open", path)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	return nil
}

// htmlExcerptBudget clips excerpts when ExcerptBudget is 0. Excerpts are the
// point of the HTML surface (the cards exist to show them), so a zero budget
// still shows text — just clipped to a generous default so one runaway source
// body cannot blow up the page.
const htmlExcerptBudget = 600

// htmlCitation is one resolved source under a marker, shaped for the JSON blob
// the inline script reads. It never carries raw markup: all strings are plain
// text and get HTML-escaped by encoding/json into the <script> block.
type htmlCitation struct {
	SourceID    string           `json:"sourceId"`
	Handle      string           `json:"handle"`   // 8-char id prefix
	Title       string           `json:"title"`    // resolved display title
	Location    string           `json:"location"` // "file:line" or "src N-M"
	Excerpt     string           `json:"excerpt"`
	ExcerptRuns []htmlExcerptRun `json:"excerptRuns,omitempty"`
	Confidence  float64          `json:"confidence"`
	HasConf     bool             `json:"hasConf"` // confidence present and shown
	Weak        bool             `json:"weak"`    // below weakConfidence
	Removed     bool             `json:"removed"` // source ID absent from the notebook source list; title unavailable
}

type htmlExcerptRun struct {
	Text string `json:"text"`
	Code bool   `json:"code,omitempty"`
	Link string `json:"link,omitempty"`
}

// htmlMarker is one [N] citation marker and the sources cited under it. Index
// is the join key that links the answer's literal [N] to this marker's entry.
//
// Span, when non-nil, is the first [start,end) range of the answer text that
// this source grounds — the reply-span, which sits before the [N] token, not at
// it. Spans carries every occurrence for server-side rendering; only Span is
// retained in the JSON payload for compatibility with existing consumers.
// The client underlines that range in the answer as the grounded passage. It is
// carried ONLY when validated in Go (in range, non-empty, not overlapping any
// [N] marker token); a zero-width point span or an out-of-range span leaves it
// nil, and the client falls back to underlining just the [N] marker.
type htmlMarker struct {
	Index   int            `json:"index"`
	Sources []htmlCitation `json:"sources"`
	Span    *htmlSpan      `json:"span,omitempty"`
	Spans   []htmlSpan     `json:"-"`
}

// htmlSpan is a validated [Start,End) range into the answer text, in the wire's
// UTF-16 code-unit space (the space citation StartChar/EndChar use). The answer
// renderer maps it to rune offsets at its single seam (translateMarkerSpans)
// before slicing []rune(content).
type htmlSpan struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

// htmlMessage is a rendered turn for the client. For assistant turns Runes is
// the answer split to runes (so JS can slice on the same offsets the server
// uses) and Markers lists every citation marker.
type htmlMessage struct {
	Role    string       `json:"role"`
	Content string       `json:"content"`
	Markers []htmlMarker `json:"markers,omitempty"`
}

// htmlPayload is the whole document as the inline script sees it.
type htmlPayload struct {
	Title    string        `json:"title"`
	Messages []htmlMessage `json:"messages"`
}

// answerTemplate is one assistant turn's pre-rendered answer body. The answer
// body is built server-side (see chat_render_html_answer.go) with html/template
// auto-escaping and shipped as HTML in the page; the inline script moves it into
// place and attaches interactivity by querying its data-* attributes. MsgIndex
// keys it back to the message so the script can match it and scope its lookups.
type answerTemplate struct {
	MsgIndex int
	Body     template.HTML
	Thinking template.HTML // server-rendered reasoning block; "" when not shown
}

// renderChatHTML writes an interactive HTML page for doc to w. The page inlines
// its own CSS and application JS. Pages containing TeX load MathJax from the
// same CDN as note HTML; math-free pages make no external requests. Every
// string derived from server data is escaped:
// the citation data travels as a JSON blob the inline script reads (never
// concatenated into markup or script), and each assistant answer body is
// server-rendered HTML built through html/template's contextual escaping.
func renderChatHTML(w io.Writer, doc chatDocument, ctx chatRenderContext) error {
	if !ctx.IncludeFollowUps {
		doc = withoutChatFollowUps(doc)
	}
	payload := buildHTMLPayload(doc, ctx)

	// The payload is emitted into a <script type="application/json"> block.
	// json.Marshal escapes <, >, & (via SetEscapeHTML, the default) so the
	// closing </script> sequence cannot be forged out of server text.
	blob, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode chat data: %w", err)
	}

	answers, err := buildAnswerTemplates(doc, ctx, payload)
	if err != nil {
		return fmt.Errorf("render answers: %w", err)
	}

	data := struct {
		Title   template.HTML    // pre-escaped by the template pipeline below
		Blob    template.JS      // JSON we generated; safe to embed as script content
		Answers []answerTemplate // server-rendered answer bodies, one per assistant turn
		HasMath bool
	}{
		Title:   template.HTML(template.HTMLEscapeString(displayTitle(doc))),
		Blob:    template.JS(blob),
		Answers: answers,
		HasMath: chatDocumentHasMath(doc),
	}
	if err := chatHTMLTemplate.Execute(w, data); err != nil {
		return fmt.Errorf("render html: %w", err)
	}
	return nil
}

var chatMarkdownCodePattern = regexp.MustCompile("(?s)```.*?```|`[^`\\n]*`")

func chatDocumentHasMath(doc chatDocument) bool {
	var body strings.Builder
	for _, message := range doc.Messages {
		if message.Role != "assistant" {
			continue
		}
		body.WriteString(chatMarkdownCodePattern.ReplaceAllString(message.Content, ""))
		body.WriteByte('\n')
	}
	return noteHasMath(body.String())
}

// buildAnswerTemplates renders each assistant turn's answer body to HTML,
// pairing it with the message index. The per-marker model comes from the payload
// already built for the cards/rail, so the grounded spans and [N] links are
// validated once and shared. User turns contribute no answer body.
func buildAnswerTemplates(doc chatDocument, ctx chatRenderContext, payload htmlPayload) ([]answerTemplate, error) {
	var out []answerTemplate
	for i, m := range doc.Messages {
		if m.Role != "assistant" {
			continue
		}
		var markers []htmlMarker
		if i < len(payload.Messages) {
			markers = payload.Messages[i].Markers
		}
		body, err := renderAnswerBody(i, m, markers)
		if err != nil {
			return nil, err
		}
		tmpl := answerTemplate{MsgIndex: i, Body: body}
		if ctx.ShowThinking && m.Thinking != "" {
			tmpl.Thinking = renderThinkingBody(m.Thinking)
		}
		out = append(out, tmpl)
	}
	return out, nil
}

// renderThinkingBody renders a reasoning trace to escaped HTML: a "Reasoning"
// label followed by the trace text. The text is HTML-escaped (it is server data)
// and its newlines are preserved by the .thinking block's white-space:pre-wrap,
// so a multi-step trace keeps its shape. Building it here — not in the client —
// keeps the escaping in html/template alongside the answer body.
func renderThinkingBody(text string) template.HTML {
	var sb strings.Builder
	sb.WriteString(`<span class="lbl">Reasoning</span>`)
	sb.WriteString(template.HTMLEscapeString(text))
	return template.HTML(sb.String())
}

// displayTitle is the page/document title, falling back through the
// conversation title to a generic label so the tab is never blank.
func displayTitle(doc chatDocument) string {
	if doc.Title != "" {
		return doc.Title
	}
	return "NotebookLM conversation"
}

// buildHTMLPayload projects doc into the client-facing model: it resolves
// titles/locations, groups citations by marker, and validates each marker's
// answer span against the message's rune length. Offset safety and the
// per-source card data are decided here, once, in Go — the script only renders.
func buildHTMLPayload(doc chatDocument, ctx chatRenderContext) htmlPayload {
	budget := ctx.ExcerptBudget
	if budget <= 0 {
		budget = htmlExcerptBudget
	}

	out := htmlPayload{Title: displayTitle(doc)}
	for _, m := range doc.Messages {
		hm := htmlMessage{Role: m.Role, Content: m.Content}
		// The reasoning trace is not shipped in the JSON blob — it is
		// server-rendered into a <template class="thinking-body"> instead (see
		// buildAnswerTemplates), so the client clones it rather than reading it
		// from data. Keeping it out of the blob avoids shipping the trace twice.
		if m.Role == "assistant" {
			hm.Markers = buildMarkers(m, ctx, budget)
		}
		out.Messages = append(out.Messages, hm)
	}
	return out
}

// buildMarkers groups a turn's citations by marker index into one htmlMarker per
// [N], preserving first-seen order. Each marker carries a validated grounded
// Span (the reply-span range in the answer the client underlines) when the
// group's citations agree on a real, in-range, marker-free range; otherwise Span
// is nil and the client underlines just the [N] token.
func buildMarkers(m chatDocMessage, ctx chatRenderContext, budget int) []htmlMarker {
	if len(m.Citations) == 0 {
		return nil
	}
	citations := alignHTMLCitations(m.Content, m.Citations)
	return buildCitationMarkers(
		citations,
		ctx,
		budget,
		utf16Len(m.Content),
		markerTokenRangesUTF16(m.Content),
	)
}

// buildCitationMarkers is the format-neutral citation-to-marker projection
// shared by chat turns and notes. Callers supply the coordinate-space length
// and the marker-token ranges appropriate to their own document model.
func buildCitationMarkers(citations []api.Citation, ctx chatRenderContext, budget, u16Len int, markerRanges [][2]int) []htmlMarker {
	if len(citations) == 0 {
		return nil
	}
	locations := ctx.citationLocations(citations)
	order, groups := groupCitationsByIndex(citations)

	markers := make([]htmlMarker, 0, len(order))
	for _, idx := range order {
		hm := htmlMarker{Index: idx}
		seenSources := make(map[string]bool)
		for _, c := range groups[idx] {
			key := c.SourceID + "\x00" + c.ParentSourceID
			if seenSources[key] {
				continue
			}
			seenSources[key] = true
			hm.Sources = append(hm.Sources, buildCitation(c, ctx, locations, budget))
		}
		hm.Spans = groundedSpans(groups[idx], u16Len, markerRanges)
		if len(hm.Spans) > 0 {
			hm.Span = &hm.Spans[0]
		}
		markers = append(markers, hm)
	}
	return markers
}

// alignHTMLCitations repairs the answer coordinates carried by streamed chat
// citations. Source mappings are ordered like the visible [N] tokens, but their
// ranges index the server's marker-free rich text. Persisted Markdown contains
// the marker tokens and formatting delimiters, so applying those offsets
// directly underlines unrelated characters farther and farther into the
// answer. Pair each mapping with its visible token, preserve the server's span
// width, and anchor the range immediately before that token.
//
// Sessions written before source mappings were decoded by citation-data index
// also carry the mapping ordinal as SourceIndex. The same ordered pairing lets
// us recover the actual indices from the token without rewriting the session.
func alignHTMLCitations(content string, citations []api.Citation) []api.Citation {
	tokens := htmlCitationTokens(content)
	occurrences := citationOccurrences(citations)
	if len(tokens) == 0 || len(tokens) != len(occurrences) {
		return citations
	}
	u16 := newUTF16RuneMap(content)
	runes := []rune(content)
	needsAlignment := false
	for i, occurrence := range occurrences {
		if occurrence.startChar < 0 || occurrence.endChar < occurrence.startChar ||
			occurrence.endChar > tokens[i].start || occurrence.endChar > utf16Len(content) {
			return citations
		}
		gap := string(runes[u16.rune(occurrence.endChar):u16.rune(tokens[i].start)])
		if strings.TrimSpace(gap) != "" {
			needsAlignment = true
		}
	}
	legacy := true
	for i, occurrence := range occurrences {
		for _, c := range citations[occurrence.start:occurrence.end] {
			if c.SourceIndex != i+1 {
				legacy = false
				break
			}
		}
	}
	if !needsAlignment && !legacy {
		return citations
	}

	out := append([]api.Citation(nil), citations...)
	for i, occurrence := range occurrences {
		group := out[occurrence.start:occurrence.end]
		indices := tokens[i].indices
		if legacy {
			if len(group) != len(indices) {
				return citations
			}
			for j := range group {
				group[j].SourceIndex = indices[j]
			}
		} else if !sameCitationIndices(group, indices) {
			return citations
		}

		if needsAlignment {
			width := occurrence.endChar - occurrence.startChar
			start, end := answerRangeBeforeToken(content, tokens[i].start, width)
			if !answerRangeHasText(content, start, end) {
				start = end
			}
			for j := range group {
				group[j].StartChar = start
				group[j].EndChar = end
			}
		}
	}
	return out
}

type htmlCitationToken struct {
	start   int // UTF-16 offset of "["
	indices []int
}

func htmlCitationTokens(content string) []htmlCitationToken {
	byteToU16 := byteToUTF16(content)
	var out []htmlCitationToken
	for _, match := range htmlMarkerRe.FindAllStringSubmatchIndex(content, -1) {
		indices, ok := citationIndices(content[match[2]:match[3]])
		if !ok {
			continue
		}
		out = append(out, htmlCitationToken{
			start:   byteToU16[match[0]],
			indices: indices,
		})
	}
	return out
}

func citationIndices(body string) ([]int, bool) {
	var out []int
	for _, part := range strings.Split(body, ",") {
		part = strings.TrimSpace(part)
		if lo, _, hi, ok := splitRange(part); ok {
			first, err1 := strconv.Atoi(lo)
			last, err2 := strconv.Atoi(hi)
			if err1 != nil || err2 != nil || last < first {
				return nil, false
			}
			for i := first; i <= last; i++ {
				out = append(out, i)
			}
			continue
		}
		index, err := strconv.Atoi(part)
		if err != nil {
			return nil, false
		}
		out = append(out, index)
	}
	return out, len(out) > 0
}

type citationOccurrence struct {
	start, end         int
	startChar, endChar int
}

// citationOccurrences returns the contiguous source-mapping groups emitted by
// the decoder. Every source cited by one visible marker shares one answer range.
func citationOccurrences(citations []api.Citation) []citationOccurrence {
	var out []citationOccurrence
	for start := 0; start < len(citations); {
		end := start + 1
		for end < len(citations) &&
			citations[end].StartChar == citations[start].StartChar &&
			citations[end].EndChar == citations[start].EndChar {
			end++
		}
		out = append(out, citationOccurrence{
			start:     start,
			end:       end,
			startChar: citations[start].StartChar,
			endChar:   citations[start].EndChar,
		})
		start = end
	}
	return out
}

func sameCitationIndices(group []api.Citation, indices []int) bool {
	if len(group) != len(indices) {
		return false
	}
	seen := make(map[int]int)
	for _, c := range group {
		seen[c.SourceIndex]++
	}
	for _, index := range indices {
		if seen[index] == 0 {
			return false
		}
		seen[index]--
	}
	return true
}

// answerRangeBeforeToken returns a UTF-16 range of width code units ending at
// the last non-space character before tokenStart. The server range width is
// stable even though its absolute offsets index marker-free rich text.
func answerRangeBeforeToken(content string, tokenStart, width int) (int, int) {
	runes := []rune(content)
	u16 := newUTF16RuneMap(content)
	endRune := u16.rune(tokenStart)
	for endRune > 0 && unicode.IsSpace(runes[endRune-1]) {
		endRune--
	}
	runeToU16 := make([]int, len(runes)+1)
	at := 0
	for i, r := range runes {
		runeToU16[i] = at
		at += utf16.RuneLen(r)
	}
	runeToU16[len(runes)] = at
	end := runeToU16[endRune]
	start := end - width
	if start < 0 {
		start = 0
	}
	return start, end
}

func answerRangeHasText(content string, start, end int) bool {
	u16 := newUTF16RuneMap(content)
	runes := []rune(content)
	for _, r := range runes[u16.rune(start):u16.rune(end)] {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			return true
		}
	}
	return false
}

// groundedSpans returns the distinct validated answer ranges for a marker's
// citations. A citation's [StartChar,EndChar) qualifies when it is
// a real range (start < end), lies within [0,u16Len], and does not overlap any
// [N] marker token (so underlining the grounded passage never swallows a marker
// the renderer also turns into a link). All bounds are in the wire's UTF-16
// code-unit space — the space StartChar/EndChar and markerRanges use — so the
// span is not mapped to runes until the render seam. When several citations
// under one marker carry the same qualifying range, it is emitted once; a
// zero-width point span or an out-of-range span is skipped.
// Underlining the grounded sentence — not the [N] — is the point: the reply-span
// sits before the marker, so it never coincides with it.
func groundedSpans(group []api.Citation, u16Len int, markerRanges [][2]int) []htmlSpan {
	var out []htmlSpan
	seen := make(map[[2]int]bool)
	for _, c := range group {
		s, e := c.StartChar, c.EndChar
		if s >= e || s < 0 || e > u16Len {
			continue
		}
		if rangeOverlapsAny(s, e, markerRanges) {
			continue
		}
		key := [2]int{s, e}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, htmlSpan{Start: s, End: e})
	}
	return out
}

// markerTokenRangesUTF16 returns the UTF-16 code-unit ranges of every [N]-style
// citation marker in text (e.g. "[2]", "[1-4]", "[1, 2, 3]"), so a grounded span
// that would overlap a marker can be rejected. Ranges are in UTF-16 offsets to
// match the citation StartChar/EndChar space (the wire counts answer offsets in
// UTF-16 units, so an emoji before a marker shifts it by one unit per surrogate
// pair).
func markerTokenRangesUTF16(text string) [][2]int {
	byteToU16 := byteToUTF16(text)
	var out [][2]int
	for _, loc := range htmlMarkerRe.FindAllStringIndex(text, -1) {
		out = append(out, [2]int{byteToU16[loc[0]], byteToU16[loc[1]]})
	}
	return out
}

// rangeOverlapsAny reports whether [s,e) intersects any of the given ranges.
func rangeOverlapsAny(s, e int, ranges [][2]int) bool {
	for _, r := range ranges {
		if s < r[1] && r[0] < e {
			return true
		}
	}
	return false
}

// htmlMarkerRe matches the server's inline citation markers — a bracketed list
// of indices or ranges (e.g. "[2]", "[1-4]", "[1, 2, 3]"). It is used both to
// find marker-token ranges (so a grounded span never overlaps one) and, in the
// answer renderer, to place the [N] links. The submatch captures the bracket
// body so the answer renderer can split it into indices.
var htmlMarkerRe = regexp.MustCompile(`\[(\d+(?:\s*[-,]\s*\d+)*)\]`)

// buildCitation shapes one source under a marker: its handle, resolved title
// and location, clipped excerpt, and per-source confidence flags (honoring
// HideConfidence and HideSpans).
func buildCitation(c api.Citation, ctx chatRenderContext, locations map[citationKey]string, budget int) htmlCitation {
	excerpt := clipExcerpt(c.Excerpt, budget)
	hc := htmlCitation{
		SourceID:    c.SourceID,
		Handle:      shortSourceID(c.SourceID),
		Title:       ctx.citationSourceTitle(c),
		Excerpt:     excerpt,
		ExcerptRuns: buildExcerptRuns(c.ExcerptRuns, c.Excerpt, excerpt, budget),
		Removed:     ctx.citationSourceRemoved(c),
	}

	if loc, ok := locations[keyFor(c)]; ok && loc != "" {
		hc.Location = loc
	} else if !ctx.HideSpans {
		hc.Location = formatSourceSpan(c.SourceStart, c.SourceEnd)
	}

	if !ctx.HideConfidence && c.Confidence > 0 {
		hc.HasConf = true
		hc.Confidence = c.Confidence
		hc.Weak = c.Confidence < weakConfidence
	}
	return hc
}

func buildExcerptRuns(runs []api.ExcerptRun, flat, clipped string, budget int) []htmlExcerptRun {
	if len(runs) == 0 || decodeNumberedExcerpt(flat) != flat || formatFlattenedExcerptTable(flat) != flat {
		return nil
	}
	var joined strings.Builder
	for _, run := range runs {
		joined.WriteString(run.Text)
	}
	if joined.String() != flat {
		return nil
	}
	trimmed := strings.TrimSpace(flat)
	if clipRunes(trimmed, budget) != clipped {
		return nil
	}
	start := len([]rune(flat[:strings.Index(flat, trimmed)]))
	end := start + len([]rune(trimmed))
	if budget < end-start {
		end = start + budget
	}

	var out []htmlExcerptRun
	offset := 0
	for _, run := range runs {
		text := []rune(run.Text)
		runStart, runEnd := offset, offset+len(text)
		offset = runEnd
		from := max(start, runStart)
		to := min(end, runEnd)
		if from >= to {
			continue
		}
		link, _ := safeExcerptLink(run.Link)
		out = append(out, htmlExcerptRun{
			Text: string(text[from-runStart : to-runStart]),
			Code: run.Code,
			Link: link,
		})
	}
	if end < start+len([]rune(trimmed)) {
		out = append(out, htmlExcerptRun{Text: "…"})
	}
	return out
}

func safeExcerptLink(link string) (string, bool) {
	link = strings.TrimSpace(link)
	if link == "" || strings.IndexFunc(link, func(r rune) bool {
		return unicode.IsControl(r) || unicode.IsSpace(r)
	}) >= 0 {
		return "", false
	}
	parsed, err := url.Parse(link)
	if err != nil {
		return "", false
	}
	switch strings.ToLower(parsed.Scheme) {
	case "", "http", "https", "mailto":
		return link, true
	default:
		return "", false
	}
}

// chatHTMLTemplate is the whole self-contained page. Content and citation data
// are supplied out-of-band as the JSON blob; this template never interpolates
// server text into markup — .Blob is the JSON we generated and .Title is
// already HTML-escaped. The inline script builds the reader from the blob.
var chatHTMLTemplate = template.Must(template.New("chat").Parse(chatHTMLSource))

const chatHTMLSource = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}}</title>
<style>
:root {
  --ground: #fbfbfd;
  --panel: #ffffff;
  --ink: #1a1c22;
  --muted: #6a6f7b;
  --faint: #9aa0ac;
  --line: #e4e6ec;
  --line-strong: #d3d7e0;
  --accent: #3a5bd0;
  --accent-tint: #eef1fc;
  --accent-strong: #26399a;
  --warn: #b8730a;
  --warn-tint: #fff4e0;
  --warn-line: #f0d199;
  --user-tint: #f1f3f8;
  --radius: 8px;
  --measure: 68ch;
  --sans: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
  --mono: ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, "Liberation Mono", monospace;
}
* { box-sizing: border-box; }
html { -webkit-text-size-adjust: 100%; }
html, body { max-width: 100%; overflow-x: hidden; }
body {
  margin: 0;
  background: var(--ground);
  color: var(--ink);
  font-family: var(--sans);
  font-size: 16px;
  line-height: 1.6;
  -webkit-font-smoothing: antialiased;
}
.wrap { max-width: 1180px; margin: 0 auto; padding: 32px 24px 96px; }
header.doc {
  display: flex; flex-direction: column; gap: 4px;
  padding-bottom: 20px; margin-bottom: 28px;
  border-bottom: 1px solid var(--line);
}
header.doc h1 { margin: 0; font-size: 22px; font-weight: 650; letter-spacing: -0.01em; text-wrap: balance; }
header.doc .sub { color: var(--muted); font-size: 13px; }

.turn { margin-bottom: 40px; }
.role {
  font-size: 11px; font-weight: 650; letter-spacing: 0.09em; text-transform: uppercase;
  color: var(--faint); margin-bottom: 10px;
}
.turn.user .role { color: var(--accent-strong); }

.bubble.user {
  max-width: var(--measure);
  background: var(--user-tint);
  border: 1px solid var(--line);
  border-radius: var(--radius);
  padding: 14px 18px;
  white-space: pre-wrap; word-wrap: break-word;
}

.assistant-grid { display: grid; grid-template-columns: minmax(0, 1fr) 340px; gap: 32px; align-items: start; }

.answer {
  max-width: var(--measure);
  min-width: 0;
  word-wrap: break-word;
  font-size: 16.5px;
}
/* Structural answer body, server-rendered from the block tree. Paragraphs,
   headings, lists and rules come back as real elements; a flat answer (no tree,
   or newline-bearing content) renders as one pre-wrapped block. */
.answer p { margin: 0 0 0.9em; }
.answer p:last-child { margin-bottom: 0; }
.answer h4 { margin: 1.1em 0 0.5em; font-size: 15px; font-weight: 650; letter-spacing: 0.01em; }
.answer ul { margin: 0 0 0.9em; padding-left: 1.4em; }
.answer li { margin: 0.15em 0; }
.answer li.nest-1 { margin-left: 1.2em; }
.answer li.nest-2 { margin-left: 2.4em; }
.answer li.nest-3 { margin-left: 3.6em; }
.answer hr { border: 0; border-top: 1px solid var(--line); margin: 1.2em 0; }
.answer .answer-block { white-space: pre-wrap; }
.answer pre, .answer table { display: block; max-width: 100%; overflow-x: auto; }
.math-display-row {
  display: grid; grid-template-columns: minmax(0,1fr) auto minmax(0,1fr);
  align-items: center; width: 100%; max-width: 100%; margin: .9em 0;
}
.math-display-row::before { content: ""; grid-column: 1; }
.math-display-equation {
  grid-column: 2; min-width: 0; max-width: 100%; overflow-x: auto;
  text-align: center;
}
.math-display-cite { grid-column: 3; justify-self: end; padding-left: .6em; }
@media (max-width: 520px) {
  .math-display-row { grid-template-columns: minmax(0,1fr) auto; }
  .math-display-row::before { display: none; }
  .math-display-equation { grid-column: 1; }
  .math-display-cite { grid-column: 2; padding-left: .35em; }
}
.thinking {
  max-width: var(--measure);
  margin-bottom: 16px; padding: 12px 16px;
  background: #f6f6f9; border: 1px solid var(--line); border-radius: var(--radius);
  color: var(--muted); font-size: 14px; white-space: pre-wrap;
}
.thinking .lbl { display:block; font-size:10px; letter-spacing:0.09em; text-transform:uppercase; color:var(--faint); margin-bottom:6px; }

/* Grounded passage: the answer text a citation grounds, underlined in place so
   the underline marks the CLAIM itself (not the [N], not the excerpt). Hovering
   it previews the marker's sources and cross-lights the [N] link and rail. */
.grounded {
  text-decoration: underline; text-decoration-color: #b9c4ef;
  text-underline-offset: 2px; text-decoration-thickness: 1.5px;
  cursor: pointer;
  transition: background 120ms ease, text-decoration-color 120ms ease;
}
.grounded:hover, .grounded.active, .grounded.flash {
  background: var(--accent-tint); text-decoration-color: var(--accent);
}

/* Inline citation marker: the server's own marker, linked down to the citation
   entry. The superscript group is compact and stays together. */
.citegroup {
  white-space: nowrap; font-size: .72em; line-height: 0;
  vertical-align: super; margin-left: .08em;
}
.citelink {
  color: var(--accent-strong); font-weight: 600;
  text-decoration: underline; text-decoration-style: dotted;
  text-decoration-color: var(--accent-strong);
  text-underline-offset: 1px; text-decoration-thickness: 1px;
  padding: 0 1px; border-radius: 3px; cursor: pointer;
  transition: background 120ms ease, color 120ms ease, text-decoration-color 120ms ease;
}
.citelink:hover, .citelink:focus-visible, .citelink.active {
  background: var(--accent-tint); color: var(--accent);
  text-decoration-color: var(--accent);
}

/* Hover/pin card raised over an inline span. */
.card {
  position: absolute; z-index: 40;
  width: min(420px, 90vw);
  background: var(--panel);
  border: 1px solid var(--line-strong);
  border-radius: 10px;
  box-shadow: 0 8px 28px rgba(24,28,40,0.16), 0 1px 3px rgba(24,28,40,0.10);
  padding: 4px; display: none;
  font-size: 14px;
}
.card.show { display: block; }
.card-close {
  appearance: none; position: sticky; top: 4px; float: right; z-index: 1;
  width: 36px; height: 36px; margin: 2px 2px 0 8px;
  border: 1px solid var(--line-strong); border-radius: 999px;
  background: var(--panel); color: var(--muted); cursor: pointer;
  font: 600 20px/1 var(--sans);
}
.card-close:hover, .card-close:focus-visible { color: var(--ink); border-color: var(--accent); }
.card .src { padding: 10px 12px; border-radius: 7px; }
.card .src + .src { border-top: 1px solid var(--line); }
.card .src-head { display: flex; align-items: baseline; gap: 8px; flex-wrap: wrap; margin-bottom: 6px; }
.card .title { font-weight: 600; }
.card .handle, .card .loc { font-family: var(--mono); font-size: 12px; color: var(--muted); }
.card .loc { margin-left: auto; }
.card .excerpt {
  font-family: var(--mono); font-size: 12.5px; line-height: 1.55;
  color: #33383f; background: #f7f8fb; border: 1px solid var(--line);
  border-radius: 6px; padding: 8px 10px; margin-top: 4px;
  max-height: 240px; overflow-y: auto; white-space: pre-wrap; word-wrap: break-word;
}

/* Confidence pill, per source; amber below weakConfidence. */
.pill {
  font-family: var(--mono); font-size: 11px; font-weight: 600;
  padding: 1px 7px; border-radius: 999px;
  background: var(--accent-tint); color: var(--accent-strong);
  font-variant-numeric: tabular-nums;
}
.pill.weak { background: var(--warn-tint); color: var(--warn); border: 1px solid var(--warn-line); }

/* A citation whose source could not be resolved to a titled notebook source.
   The handle is all we have; label it so a blank title reads as "untitled",
   not a rendering gap. */
.removed {
  font-size: 11px; font-weight: 600;
  padding: 1px 7px; border-radius: 999px;
  background: #f2f3f5; color: var(--muted);
  border: 1px dashed var(--line-strong);
}

/* Sources rail beside the answer: an at-a-glance index. */
.rail { position: sticky; top: 20px; display: flex; flex-direction: column; gap: 10px; max-height: calc(100vh - 40px); overflow-y: auto; }
.rail .rail-head { font-size: 11px; font-weight: 650; letter-spacing: 0.09em; text-transform: uppercase; color: var(--faint); }
.rail .empty { color: var(--faint); font-size: 13px; font-style: italic; }
.ref {
  border: 1px solid var(--line); border-radius: var(--radius);
  background: var(--panel); padding: 10px 12px; cursor: pointer;
  transition: border-color 120ms ease, box-shadow 120ms ease;
}
.ref:hover, .ref.active { border-color: var(--accent); box-shadow: 0 0 0 3px var(--accent-tint); }
.ref-head { display: flex; align-items: baseline; gap: 8px; margin-bottom: 6px; }
.ref-marker {
  font-size: 12px; font-weight: 700; color: var(--accent-strong);
  min-width: 22px; height: 20px; padding: 0 5px;
  display: inline-flex; align-items: center; justify-content: center;
  background: var(--accent-tint); border-radius: 5px;
}
.ref-title { font-weight: 600; font-size: 14px; overflow-wrap: anywhere; flex: 1; }
/* The cited source text in the rail. NOT underlined — the underline belongs to
   the grounded passage in the answer; here the excerpt reads as a quoted preview
   (left rule) of the source. Hovering the entry previews the full card. */
.ref-excerpt {
  display: block; margin-top: 5px;
  font-family: var(--mono); font-size: 12px; line-height: 1.5; color: #4a4f59;
  padding-left: 8px; border-left: 2px solid var(--line-strong);
  display: -webkit-box; -webkit-line-clamp: 3; -webkit-box-orient: vertical; overflow: hidden;
}
.ref:hover .ref-excerpt, .ref.active .ref-excerpt { border-left-color: var(--accent); }
.ref-excerpt + .ref-excerpt { margin-top: 6px; padding-top: 6px; border-top: 1px solid var(--line); }
.ref-actions { display: flex; gap: 7px; margin-top: 9px; }
.ref-action {
  appearance: none; border: 1px solid var(--line-strong); border-radius: 6px;
  background: var(--panel); color: var(--accent-strong); cursor: pointer;
  font: 600 12px/1.2 var(--sans); padding: 6px 9px; text-decoration: none;
}
.ref-action:hover, .ref-action:focus-visible {
  border-color: var(--accent); background: var(--accent-tint);
}
.ref-action:disabled { color: var(--faint); cursor: default; background: #f7f7f8; }

/* Citations section at the bottom of an assistant turn. */
.citations {
  margin-top: 28px; padding-top: 18px;
  border-top: 1px solid var(--line);
}
.citations-head {
  margin: 0 0 14px; font-size: 12px; font-weight: 650;
  letter-spacing: 0.09em; text-transform: uppercase; color: var(--faint);
}
.cite-entry {
  display: grid; grid-template-columns: 44px 1fr; gap: 8px;
  padding: 10px 8px; border-radius: var(--radius);
  scroll-margin-top: 20px;
  transition: background 500ms ease;
}
.cite-entry.flash { background: var(--accent-tint); }
.cite-num {
  font-family: var(--mono); font-size: 13px; font-weight: 700;
  color: var(--accent-strong); padding-top: 1px;
}
.cite-src { padding: 4px 0; }
.cite-src + .cite-src { margin-top: 8px; padding-top: 10px; border-top: 1px solid var(--line); }
.cite-src-head { display: flex; align-items: baseline; gap: 8px; flex-wrap: wrap; margin-bottom: 5px; }
.cite-src-head .title { font-weight: 600; font-size: 14px; overflow-wrap: anywhere; }
.cite-src-head .handle, .cite-src-head .loc { font-family: var(--mono); font-size: 12px; color: var(--muted); }
.cite-src .excerpt {
  font-family: var(--mono); font-size: 12.5px; line-height: 1.55;
  color: #33383f; background: #f7f8fb; border: 1px solid var(--line);
  border-radius: 6px; padding: 8px 10px; margin-top: 4px;
  max-height: 260px; overflow-y: auto; white-space: pre-wrap; word-wrap: break-word;
}
:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
@media (max-width: 860px) {
  .wrap { max-width: 100%; padding: 24px 18px 72px; }
  .assistant-grid { grid-template-columns: minmax(0, 1fr); gap: 22px; }
  .rail {
    position: static; top: auto; max-height: none; overflow: visible;
    padding-top: 18px; border-top: 1px solid var(--line);
  }
}
@media (max-width: 520px) {
  .wrap { padding: 18px 12px 56px; }
  header.doc { margin-bottom: 22px; }
  header.doc h1 { font-size: 20px; }
  .turn { margin-bottom: 32px; }
  .bubble.user { padding: 12px 14px; }
  .answer { font-size: 16px; overflow-wrap: anywhere; }
  .cite-entry { grid-template-columns: 34px minmax(0, 1fr); padding-inline: 2px; }
  .cite-src-head .loc { width: 100%; }
  .card {
    position: fixed; left: 10px !important; right: 10px; top: auto !important;
    bottom: max(10px, env(safe-area-inset-bottom)); width: auto;
    max-height: min(72vh, 34rem); overflow-y: auto;
  }
  .card-close { width: 44px; height: 44px; }
  .ref-action { min-height: 44px; display: inline-flex; align-items: center; }
}
@media (hover: none), (pointer: coarse) {
  .citelink, .grounded { position: relative; }
  .citelink::after, .grounded::after {
    content: ""; position: absolute; z-index: 1;
    left: -8px; right: -8px; top: 50%; height: 44px; transform: translateY(-50%);
  }
}
@media (prefers-reduced-motion: reduce) { * { transition: none !important; } }
</style>
</head>
<body>
<div class="wrap" id="root"></div>
<div class="card" id="card" role="dialog" aria-modal="false" aria-label="Citation sources" tabindex="-1"></div>
<script id="chat-data" type="application/json">{{.Blob}}</script>
{{- range .Answers}}
<template class="answer-body" data-msg="{{.MsgIndex}}">{{.Body}}</template>
{{- if .Thinking}}
<template class="thinking-body" data-msg="{{.MsgIndex}}">{{.Thinking}}</template>
{{- end}}
{{- end}}
<script>
(function () {
  "use strict";
  var data;
  try {
    data = JSON.parse(document.getElementById("chat-data").textContent);
  } catch (e) {
    document.getElementById("root").textContent = "Failed to load chat data.";
    return;
  }
  var root = document.getElementById("root");
  var card = document.getElementById("card");
  var touchQuery = window.matchMedia("(hover: none), (pointer: coarse)");

  function el(tag, cls, text) {
    var n = document.createElement(tag);
    if (cls) n.className = cls;
    if (text != null) n.textContent = text;
    return n;
  }
  function appendExcerpt(parent, citation) {
    if (!citation.excerptRuns || !citation.excerptRuns.length) {
      parent.textContent = citation.excerpt || "";
      return;
    }
    citation.excerptRuns.forEach(function (run) {
      var content = run.code ? el("code", "", run.text) : document.createTextNode(run.text);
      if (!run.link) {
        parent.appendChild(content);
        return;
      }
      var anchor = document.createElement("a");
      anchor.href = run.link;
      anchor.target = "_blank";
      anchor.rel = "noopener noreferrer";
      anchor.appendChild(content);
      parent.appendChild(anchor);
    });
  }
  function fmtConf(c) { return "p=" + c.confidence.toFixed(2); }

  function confPill(c) {
    if (!c.hasConf) return null;
    var p = el("span", "pill" + (c.weak ? " weak" : ""), fmtConf(c));
    p.title = c.weak ? "weak confidence" : "confidence";
    return p;
  }

  // An untitled-source badge, shown next to the handle when the citation's
  // source could not be resolved to a titled notebook source. A citation handle
  // is a granular chunk/passage ID, so it misses the notebook's source list even
  // when the source is present — the honest statement is that the title is
  // unavailable, not that the source was removed.
  function removedTag(c) {
    if (!c.removed || c.title) return null;
    var t = el("span", "removed", "untitled");
    t.title = "This citation's source could not be resolved to a titled notebook source. Its title is unavailable (the source may have been re-synced, or the title lookup failed — try re-running with fresh auth).";
    return t;
  }

  // Build the card body for a marker's sources.
  function fillCard(marker) {
    card.textContent = "";
    var close = el("button", "card-close", "×");
    close.type = "button";
    close.setAttribute("aria-label", "Close citation preview");
    close.addEventListener("click", function (event) {
      event.stopPropagation();
      closeCard();
    });
    card.appendChild(close);
    marker.sources.forEach(function (c) {
      var src = el("div", "src");
      var head = el("div", "src-head");
      if (c.title) head.appendChild(el("span", "title", c.title));
      if (c.handle) head.appendChild(el("span", "handle", c.handle));
      var rm = removedTag(c);
      if (rm) head.appendChild(rm);
      var pill = confPill(c);
      if (pill) head.appendChild(pill);
      if (c.location) head.appendChild(el("span", "loc", c.location));
      src.appendChild(head);
      if (c.excerpt) {
        var excerpt = el("div", "excerpt");
        appendExcerpt(excerpt, c);
        src.appendChild(excerpt);
      }
      card.appendChild(src);
    });
  }

  function positionCard(anchor) {
    card.classList.add("show");
    var r = anchor.getBoundingClientRect();
    var cw = card.offsetWidth, ch = card.offsetHeight;
    var left = window.scrollX + r.left;
    left = Math.min(left, window.scrollX + document.documentElement.clientWidth - cw - 12);
    left = Math.max(window.scrollX + 12, left);
    var top = window.scrollY + r.bottom + 6;
    // Flip above the span if it would overflow the viewport bottom.
    if (r.bottom + ch + 12 > window.innerHeight && r.top - ch - 6 > 0) {
      top = window.scrollY + r.top - ch - 6;
    }
    card.style.left = left + "px";
    card.style.top = top + "px";
  }

  // The card is a lightweight hover PREVIEW of a marker's sources; the canonical,
  // always-present view is the Citations section the [N] links jump to. So the
  // card needs no pinning or focus trapping — it appears on hover/focus of an
  // inline link and dismisses when the pointer leaves. A short close delay lets
  // the pointer cross the gap into the card to scroll a long excerpt.
  var spanEls = {};   // markerKey -> grounded passages and inline [N] links
  var groundEls = {}; // markerKey -> grounded passages, in document order
  var railEls = {};   // markerKey -> rail entry element
  function keyOf(msgIdx, markerIdx) { return msgIdx + ":" + markerIdx; }

  var activeKey = null;
  // setActive cross-lights a marker's inline [N] link(s) and its rail entry, so
  // hovering either surface highlights both — the reply_span ↔ [N] ↔ rail link
  // made visible.
  function setActive(key) {
    if (activeKey === key) return;
    clearActive();
    activeKey = key;
    (spanEls[key] || []).forEach(function (s) { s.classList.add("active"); });
    if (railEls[key]) railEls[key].classList.add("active");
  }
  function clearActive() {
    if (activeKey == null) return;
    (spanEls[activeKey] || []).forEach(function (s) { s.classList.remove("active"); });
    if (railEls[activeKey]) railEls[activeKey].classList.remove("active");
    activeKey = null;
  }

  var hideTimer = null;
  var pinnedAnchor = null;
  function showCard(anchor, marker, key) {
    if (hideTimer) { clearTimeout(hideTimer); hideTimer = null; }
    fillCard(marker);
    positionCard(anchor);
    if (key != null) setActive(key);
  }
  function hideCard() {
    if (pinnedAnchor) return;
    if (hideTimer) clearTimeout(hideTimer);
    hideTimer = setTimeout(function () {
      hideTimer = null;
      card.classList.remove("show");
      clearActive();
    }, 140);
  }
  function closeCard() {
    if (hideTimer) { clearTimeout(hideTimer); hideTimer = null; }
    pinnedAnchor = null;
    card.classList.remove("show");
    clearActive();
  }
  // touchPreview pins a preview on the first tap. A second tap closes it and
  // falls through to the element's native action (for a citation link, its
  // href; for a rail entry, its detail jump).
  function touchPreview(event, anchor, marker, key) {
    if (!touchQuery.matches) return false;
    if (pinnedAnchor === anchor && card.classList.contains("show")) {
      closeCard();
      return false;
    }
    event.preventDefault();
    event.stopPropagation();
    pinnedAnchor = anchor;
    showCard(anchor, marker, key);
    return true;
  }
  card.addEventListener("mouseenter", function () {
    if (hideTimer) { clearTimeout(hideTimer); hideTimer = null; }
  });
  card.addEventListener("mouseleave", function () { hideCard(); });
  card.addEventListener("click", function (event) { event.stopPropagation(); });

  // Adopt the server-rendered answer body for a turn and wire its interactivity.
  //
  // The answer DOM — paragraphs, lists, the underlined grounded spans, and the
  // [N] citelink anchors — is built in Go with html/template auto-escaping and
  // shipped as the content of a <template class="answer-body" data-msg="N">. Here
  // we clone it into the page and attach the hover/highlight behavior by QUERYING
  // the rendered elements via their data-* attributes, mapping each element's
  // data-cite back to its marker so the card can preview that marker's sources.
  // No answer text is constructed in JS, so no server text is set as markup.
  function renderAnswer(msgIdx, msg) {
    var known = {};
    (msg.markers || []).forEach(function (m) { known[m.index] = m; });

    var container = el("div", "answer");
    var tpl = document.querySelector('template.answer-body[data-msg="' + msgIdx + '"]');
    if (tpl) container.appendChild(tpl.content.cloneNode(true));

    // Grounded passages: underline hover-previews the marker's sources.
    container.querySelectorAll('.grounded[data-cite]').forEach(function (s) {
      var marker = known[parseInt(s.getAttribute("data-cite"), 10)];
      if (!marker) return;
      var key = keyOf(msgIdx, marker.index);
      s.setAttribute("aria-label", "Grounded by citation " + marker.index);
      s.addEventListener("mouseenter", function () { showCard(s, marker, key); });
      s.addEventListener("mouseleave", function () { hideCard(); });
      s.addEventListener("click", function (event) { touchPreview(event, s, marker, key); });
      (spanEls[key] || (spanEls[key] = [])).push(s);
      (groundEls[key] || (groundEls[key] = [])).push(s);
    });

    // Inline [N] links: a plain anchor jump to the citation entry, plus a
    // hover/focus card preview. The href and data-* are already set by the server
    // render; we only attach the behavior.
    container.querySelectorAll('.citelink[data-cite]').forEach(function (a) {
      var marker = known[parseInt(a.getAttribute("data-cite"), 10)];
      if (!marker) return;
      var key = keyOf(msgIdx, marker.index);
      a.setAttribute("aria-label", "Jump to citation " + marker.index);
      wireLink(a, msgIdx, marker);
      (spanEls[key] || (spanEls[key] = [])).push(a);
    });

    return container;
  }

  function citeId(msgIdx, idx) { return "cite-" + msgIdx + "-" + idx; }

  // wireLink attaches a hover/focus preview to an inline [N] link. The click is a
  // plain anchor jump to the citation entry (the href), so the citation is
  // reachable with or without JS; hover/focus raises the card only as a
  // convenience preview. No pinning — the entry it jumps to shows every source —
  // so the link stays a simple, keyboard-native anchor.
  function wireLink(a, msgIdx, marker) {
    var key = keyOf(msgIdx, marker.index);
    a.setAttribute("aria-haspopup", "dialog");
    a.addEventListener("mouseenter", function () { showCard(a, marker, key); });
    a.addEventListener("mouseleave", function () { hideCard(); });
    a.addEventListener("focus", function () { showCard(a, marker, key); });
    a.addEventListener("blur", function () { hideCard(); });
    a.addEventListener("click", function (event) {
      if (touchPreview(event, a, marker, key)) return;
      flashEntry(document.getElementById(citeId(msgIdx, marker.index)));
    });
  }

  // Build one numbered citation entry for the bottom Citations section. It is the
  // jump target for the inline [N] links (id = citeId) and lists every source
  // under the marker — handle, title, confidence (amber when weak), location, and
  // excerpt — since the bottom section has room the side rail did not.
  function citationEntry(msgIdx, marker) {
    var entry = el("div", "cite-entry");
    entry.id = citeId(msgIdx, marker.index);

    var marker_lbl = el("div", "cite-num");
    marker_lbl.textContent = "[" + marker.index + "]";
    entry.appendChild(marker_lbl);

    var body = el("div", "cite-body");
    marker.sources.forEach(function (c) {
      var src = el("div", "cite-src");
      var head = el("div", "cite-src-head");
      if (c.title) head.appendChild(el("span", "title", c.title));
      if (c.handle) head.appendChild(el("span", "handle", c.handle));
      var rm = removedTag(c);
      if (rm) head.appendChild(rm);
      var pill = confPill(c);
      if (pill) head.appendChild(pill);
      if (c.location) head.appendChild(el("span", "loc", c.location));
      src.appendChild(head);
      if (c.excerpt) {
        var excerpt = el("div", "excerpt");
        appendExcerpt(excerpt, c);
        src.appendChild(excerpt);
      }
      body.appendChild(src);
    });
    entry.appendChild(body);
    return entry;
  }

  // Build a compact rail entry for a marker: its [N], the sources' titles, and a
  // per-source excerpt line. The entry and its Details action jump to the full
  // citation; Passage jumps to the first grounded answer span. Both explicit
  // controls are native keyboard and touch targets.
  function railEntry(msgIdx, marker) {
    var key = keyOf(msgIdx, marker.index);
    var entry = el("div", "ref");
    var head = el("div", "ref-head");
    head.appendChild(el("span", "ref-marker", "[" + marker.index + "]"));
    var primary = marker.sources[0] || {};
    head.appendChild(el("span", "ref-title", primary.title || primary.handle || "source"));
    var rm = removedTag(primary);
    if (rm) head.appendChild(rm);
    var pill = confPill(primary);
    if (pill) head.appendChild(pill);
    entry.appendChild(head);

    marker.sources.forEach(function (c) {
      if (!c.excerpt) return;
      // The cited source text: underlined + hoverable, the passage that grounds
      // this marker. Hovering it raises the same preview card.
      var ex = el("span", "ref-excerpt");
      appendExcerpt(ex, c);
      ex.title = (c.title || c.handle || "source") + (c.hasConf ? " · p=" + c.confidence.toFixed(2) : "");
      entry.appendChild(ex);
    });

    var actions = el("div", "ref-actions");
    var detail = el("a", "ref-action", "Details");
    detail.href = "#" + citeId(msgIdx, marker.index);
    detail.setAttribute("aria-label", "Jump to citation " + marker.index + " details");
    detail.addEventListener("click", function (ev) {
      ev.stopPropagation();
      ev.preventDefault();
      jumpTo(citeId(msgIdx, marker.index));
    });
    actions.appendChild(detail);

    var passage = el("button", "ref-action", "Passage");
    passage.type = "button";
    passage.setAttribute("aria-label", "Jump to passage grounded by citation " + marker.index);
    if (!(groundEls[key] || []).length) {
      passage.disabled = true;
      passage.title = "No grounded passage is available";
    }
    passage.addEventListener("click", function (ev) {
      ev.stopPropagation();
      jumpToPassage(key);
    });
    actions.appendChild(passage);
    entry.appendChild(actions);

    entry.tabIndex = 0;
    entry.setAttribute("aria-label", "Jump to citation " + marker.index + " details");
    entry.addEventListener("mouseenter", function () { showCard(entry, marker, key); });
    entry.addEventListener("mouseleave", function () { hideCard(); });
    entry.addEventListener("focus", function () { showCard(entry, marker, key); });
    entry.addEventListener("blur", function () { hideCard(); });
    entry.addEventListener("click", function (event) {
      if (touchPreview(event, entry, marker, key)) return;
      jumpTo(citeId(msgIdx, marker.index));
    });
    entry.addEventListener("keydown", function (ev) {
      if (ev.key === "Enter" || ev.key === " ") { ev.preventDefault(); jumpTo(citeId(msgIdx, marker.index)); }
    });
    railEls[key] = entry;
    return entry;
  }

  // jumpTo scrolls a citation entry into view and flashes it, and lights its rail
  // entry — the shared behavior for an inline [N] click, a rail click, and a
  // keyboard activation.
  function jumpTo(id) {
    var target = document.getElementById(id);
    if (!target) return;
    target.scrollIntoView({ block: "center", behavior: "smooth" });
    flashEntry(target);
  }

  // jumpToPassage uses the first grounded span when a marker has several. The
  // complete set remains cross-lit on hover; choosing one deterministic target
  // keeps repeated taps and keyboard activation predictable.
  function jumpToPassage(key) {
    var target = (groundEls[key] || [])[0];
    if (!target) return;
    target.scrollIntoView({ block: "center", behavior: "smooth" });
    target.classList.remove("flash");
    void target.offsetWidth;
    target.classList.add("flash");
    setTimeout(function () { target.classList.remove("flash"); }, 1200);
  }

  function render() {
    var header = el("header", "doc");
    header.appendChild(el("h1", null, data.title || "NotebookLM conversation"));
    var count = data.messages.length;
    header.appendChild(el("div", "sub", count + " message" + (count === 1 ? "" : "s")));
    root.appendChild(header);

    data.messages.forEach(function (msg, msgIdx) {
      var turn = el("div", "turn " + (msg.role === "user" ? "user" : "assistant"));
      turn.appendChild(el("div", "role", msg.role === "user" ? "You" : "Assistant"));

      if (msg.role === "user") {
        turn.appendChild(el("div", "bubble user", msg.content));
        root.appendChild(turn);
        return;
      }

      var markers = (msg.markers || []).slice().sort(function (a, b) { return a.index - b.index; });

      // Two-column: answer on the left, a sticky Sources rail on the right.
      var grid = el("div", "assistant-grid");
      var main = el("div");
      // The reasoning trace is server-rendered (escaped) into a
      // <template class="thinking-body">; clone it when present, same as the
      // answer body. Only emitted when --thinking was set.
      var thinkTpl = document.querySelector('template.thinking-body[data-msg="' + msgIdx + '"]');
      if (thinkTpl) {
        var think = el("div", "thinking");
        think.appendChild(thinkTpl.content.cloneNode(true));
        main.appendChild(think);
      }
      main.appendChild(renderAnswer(msgIdx, msg));
      grid.appendChild(main);

      var rail = el("div", "rail");
      rail.appendChild(el("div", "rail-head", "Sources"));
      if (markers.length === 0) {
        rail.appendChild(el("div", "empty", "No citations for this turn."));
      } else {
        markers.forEach(function (m) { rail.appendChild(railEntry(msgIdx, m)); });
      }
      grid.appendChild(rail);
      turn.appendChild(grid);

      // Full Citations section at the bottom (jump target for [N] and rail).
      if (markers.length > 0) {
        var section = el("section", "citations");
        section.setAttribute("aria-label", "Citations");
        section.appendChild(el("h2", "citations-head", "Citations"));
        markers.forEach(function (m) { section.appendChild(citationEntry(msgIdx, m)); });
        turn.appendChild(section);
      }
      root.appendChild(turn);
    });
  }

  // Esc and tap-away dismiss a pinned touch preview.
  document.addEventListener("keydown", function (ev) {
    if (ev.key === "Escape") closeCard();
  });
  document.addEventListener("click", function (event) {
    if (pinnedAnchor && !card.contains(event.target)) closeCard();
  });

  // flashEntry briefly highlights a citation entry so the eye lands on the right
  // one after a jump.
  function flashEntry(target) {
    if (!target || !target.classList.contains("cite-entry")) return;
    target.classList.remove("flash");
    void target.offsetWidth; // restart the transition
    target.classList.add("flash");
    setTimeout(function () { target.classList.remove("flash"); }, 1200);
  }
  // An inline [N] link's default anchor jump (the href) sets the hash; flash the
  // target so a jump via keyboard or a copied link still lands visibly.
  window.addEventListener("hashchange", function () {
    flashEntry(document.getElementById(location.hash.slice(1)));
  });

  render();
})();
</script>
{{if .HasMath}}<!-- MathJax support -->
<script id="MathJax-script" async src="https://cdn.jsdelivr.net/npm/mathjax@3/es5/tex-mml-chtml.js"></script>
<script>
    window.MathJax = {
        tex: {
            inlineMath: [['$', '$'], ['\\(', '\\)']],
            displayMath: [['$$', '$$'], ['\\[', '\\]']]
        }
    };
</script>
{{end}}
<!-- MathJax is loaded from the CDN for local HTML. An offline or
claude.ai Artifact variant would need to inline the runtime. -->
</body>
</html>
`
