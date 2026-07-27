package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"

	"github.com/tmc/nlm/internal/notebooklm/api"
)

// renderChatHTMLToDestination renders doc to a self-contained HTML page and
// writes it to opts.OutFile (or os.Stdout when empty), optionally opening the
// written file in a browser. The HTML build itself lives in renderChatHTML so
// it can be tested against a bytes.Buffer with no filesystem or exec.
func renderChatHTMLToDestination(doc chatDocument, ctx chatRenderContext, opts chatRenderOptions) error {
	if opts.OutFile == "" {
		return renderChatHTML(os.Stdout, doc, ctx)
	}

	var buf bytes.Buffer
	if err := renderChatHTML(&buf, doc, ctx); err != nil {
		return err
	}
	if err := os.WriteFile(opts.OutFile, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("write html: %w", err)
	}
	fmt.Fprintf(os.Stderr, "nlm: wrote %s\n", opts.OutFile)

	// --open with no file is rejected upstream; guard defensively so a stray
	// call never tries to open stdout.
	if opts.Open {
		if err := openInBrowser(opts.OutFile); err != nil {
			fmt.Fprintf(os.Stderr, "nlm: could not open browser: %v\n", err)
		}
	}
	return nil
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
	SourceID   string  `json:"sourceId"`
	Handle     string  `json:"handle"`   // 8-char id prefix
	Title      string  `json:"title"`    // resolved display title
	Location   string  `json:"location"` // "file:line" or "src N-M"
	Excerpt    string  `json:"excerpt"`
	Confidence float64 `json:"confidence"`
	HasConf    bool    `json:"hasConf"` // confidence present and shown
	Weak       bool    `json:"weak"`    // below weakConfidence
	Removed    bool    `json:"removed"` // source ID absent from the notebook source list; title unavailable
}

// htmlMarker is one [N] citation marker and the sources cited under it. Index
// is the join key that links the answer's literal [N] to this marker's entry.
//
// Span, when non-nil, is the [start,end) rune range of the answer text that this
// marker grounds — the reply-span, which sits before the [N] token, not at it.
// The client underlines that range in the answer as the grounded passage. It is
// carried ONLY when validated in Go (in range, non-empty, not overlapping any
// [N] marker token); a zero-width point span or an out-of-range span leaves it
// nil, and the client falls back to underlining just the [N] marker.
type htmlMarker struct {
	Index   int            `json:"index"`
	Sources []htmlCitation `json:"sources"`
	Span    *htmlSpan      `json:"span,omitempty"`
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

// renderChatHTML writes a self-contained interactive HTML page for doc to w.
// The page inlines all CSS and JS and makes no external requests, so it works
// from a file:// URL offline. Every string derived from server data is escaped:
// the citation data travels as a JSON blob the inline script reads (never
// concatenated into markup or script), and each assistant answer body is
// server-rendered HTML built through html/template's contextual escaping.
func renderChatHTML(w io.Writer, doc chatDocument, ctx chatRenderContext) error {
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
	}{
		Title:   template.HTML(template.HTMLEscapeString(displayTitle(doc))),
		Blob:    template.JS(blob),
		Answers: answers,
	}
	if err := chatHTMLTemplate.Execute(w, data); err != nil {
		return fmt.Errorf("render html: %w", err)
	}
	return nil
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
	locations := ctx.citationLocations(m.Citations)
	order, groups := groupCitationsByIndex(m.Citations)

	// Citation StartChar/EndChar and marker-token bounds live in the wire's
	// UTF-16 code-unit space; validate the grounded span in that same space
	// (not rune space) so an answer with astral-plane characters neither
	// over- nor under-rejects. The span is mapped to rune offsets later, at the
	// single render seam (translateMarkerSpans).
	u16Len := utf16Len(m.Content)
	markerRanges := markerTokenRangesUTF16(m.Content)

	markers := make([]htmlMarker, 0, len(order))
	for _, idx := range order {
		hm := htmlMarker{Index: idx}
		for _, c := range groups[idx] {
			hm.Sources = append(hm.Sources, buildCitation(c, ctx, locations, budget))
		}
		hm.Span = groundedSpan(groups[idx], u16Len, markerRanges)
		markers = append(markers, hm)
	}
	return markers
}

// groundedSpan returns the validated answer range for a marker's citations, or
// nil when none qualifies. A citation's [StartChar,EndChar) qualifies when it is
// a real range (start < end), lies within [0,u16Len], and does not overlap any
// [N] marker token (so underlining the grounded passage never swallows a marker
// the client also turns into a link). All bounds are in the wire's UTF-16
// code-unit space — the space StartChar/EndChar and markerRanges use — so the
// span is not mapped to runes until the render seam. When several citations
// under one marker carry the same qualifying range (the common case), that range
// is used; a zero-width point span or an out-of-range span is skipped.
// Underlining the grounded sentence — not the [N] — is the point: the reply-span
// sits before the marker, so it never coincides with it.
func groundedSpan(group []api.Citation, u16Len int, markerRanges [][2]int) *htmlSpan {
	for _, c := range group {
		s, e := c.StartChar, c.EndChar
		if s >= e || s < 0 || e > u16Len {
			continue
		}
		if rangeOverlapsAny(s, e, markerRanges) {
			continue
		}
		return &htmlSpan{Start: s, End: e}
	}
	return nil
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
	hc := htmlCitation{
		SourceID: c.SourceID,
		Handle:   shortSourceID(c.SourceID),
		Title:    ctx.citationSourceTitle(c),
		Excerpt:  clipExcerpt(c.Excerpt, budget),
		Removed:  ctx.citationSourceRemoved(c),
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
@media (max-width: 860px) { .assistant-grid { grid-template-columns: 1fr; } }

.answer {
  max-width: var(--measure);
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
.grounded:hover, .grounded.active {
  background: var(--accent-tint); text-decoration-color: var(--accent);
}

/* Inline [N] marker link: the server's own marker, the anchor down to the
   citation entry. Underlined like every reference on the page; a dotted underline
   keeps it distinct from the grounded passage's solid one (the reference points
   AT a source; the grounded span IS the claim). */
.citegroup { white-space: nowrap; }
.citelink {
  color: var(--accent-strong); font-weight: 600;
  text-decoration: underline; text-decoration-style: dotted;
  text-decoration-color: var(--accent-strong);
  text-underline-offset: 2px; text-decoration-thickness: 1.5px;
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

  function el(tag, cls, text) {
    var n = document.createElement(tag);
    if (cls) n.className = cls;
    if (text != null) n.textContent = text;
    return n;
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
      if (c.excerpt) src.appendChild(el("div", "excerpt", c.excerpt));
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
  var spanEls = {};   // markerKey -> [inline [N] link elements]
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
  function showCard(anchor, marker, key) {
    if (hideTimer) { clearTimeout(hideTimer); hideTimer = null; }
    fillCard(marker);
    positionCard(anchor);
    if (key != null) setActive(key);
  }
  function hideCard() {
    if (hideTimer) clearTimeout(hideTimer);
    hideTimer = setTimeout(function () {
      hideTimer = null;
      card.classList.remove("show");
      clearActive();
    }, 140);
  }
  card.addEventListener("mouseenter", function () {
    if (hideTimer) { clearTimeout(hideTimer); hideTimer = null; }
  });
  card.addEventListener("mouseleave", function () { hideCard(); });

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
      (spanEls[key] || (spanEls[key] = [])).push(s);
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
    a.addEventListener("click", function () { flashEntry(document.getElementById(citeId(msgIdx, marker.index))); });
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
      if (c.excerpt) src.appendChild(el("div", "excerpt", c.excerpt));
      body.appendChild(src);
    });
    entry.appendChild(body);
    return entry;
  }

  // Build a compact rail entry for a marker: its [N], the sources' titles, and a
  // per-source excerpt line. Hovering the entry (or its excerpt) previews the
  // full card and lights the matching inline [N]; clicking jumps to the bottom
  // entry. This is the at-a-glance index beside the answer; the bottom section is
  // the full read.
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
      var ex = el("span", "ref-excerpt", c.excerpt);
      ex.title = (c.title || c.handle || "source") + (c.hasConf ? " · p=" + c.confidence.toFixed(2) : "");
      entry.appendChild(ex);
    });

    entry.tabIndex = 0;
    entry.addEventListener("mouseenter", function () { showCard(entry, marker, key); });
    entry.addEventListener("mouseleave", function () { hideCard(); });
    entry.addEventListener("focus", function () { showCard(entry, marker, key); });
    entry.addEventListener("blur", function () { hideCard(); });
    entry.addEventListener("click", function () { jumpTo(citeId(msgIdx, marker.index)); });
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

  // Esc dismisses the hover preview.
  document.addEventListener("keydown", function (ev) {
    if (ev.key === "Escape") card.classList.remove("show");
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
</body>
</html>
`
