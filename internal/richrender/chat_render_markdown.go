package richrender

import (
	"fmt"
	"io"
	"strings"

	"github.com/tmc/nlm/internal/notebooklm/api"
)

// renderChatMarkdown writes the conversation as plain CommonMark: no ANSI, no
// HTML, so it survives being pasted into an issue or doc. Each turn gets a
// #### USER / #### ASSISTANT header followed by its content. Assistant turns
// with citations get a "Citations" section: a scan-view table when
// ctx.ExcerptBudget == 0, or source-first audit blockquotes when it is > 0.
// See docs/dev/citation-rendering.md §5 for the target shapes.
func renderChatMarkdown(out io.Writer, doc ChatDocument, ctx RenderContext) error {
	bw := &markdownWriter{w: out}
	for i, m := range doc.Messages {
		if i > 0 {
			bw.blank()
		}
		bw.linef("#### %s", strings.ToUpper(m.Role))
		bw.blank()
		if ctx.ShowThinking && m.Thinking != "" {
			bw.line(collapseWhitespace(m.Thinking))
			bw.blank()
		}
		bw.line(m.Content)
		if m.Role == "assistant" && len(m.Citations) > 0 {
			bw.blank()
			if ctx.ExcerptBudget > 0 {
				renderMarkdownAudit(bw, m.Citations, ctx)
			} else {
				renderMarkdownScan(bw, m.Citations, ctx)
			}
		}
	}
	return bw.err
}

// renderMarkdownScan writes the marker-first citation table. One row per source;
// the # and answer cells are blank on a marker's 2nd+ source rows so a
// multi-source marker reads as one group. Confidence and the answer span honor
// HideConfidence/HideSpans.
func renderMarkdownScan(bw *markdownWriter, cites []api.Citation, ctx RenderContext) {
	bw.line("#### Citations")
	bw.blank()

	header := []string{"#"}
	aligns := []string{"--:"}
	if !ctx.HideSpans {
		header = append(header, "answer")
		aligns = append(aligns, ":--")
	}
	if !ctx.HideConfidence {
		header = append(header, "p")
		aligns = append(aligns, ":--")
	}
	header = append(header, "source")
	aligns = append(aligns, ":--")

	bw.line("| " + strings.Join(header, " | ") + " |")
	bw.line("|" + strings.Join(aligns, "|") + "|")

	order, groups := groupCitationsByIndex(cites)
	for _, idx := range order {
		group := groups[idx]
		answer := ""
		if start, end, ok := uniformSpan(group); ok {
			answer = spanRange(start, end)
		}
		for row, c := range group {
			cells := []string{""}
			if row == 0 {
				cells[0] = fmt.Sprintf("%d", idx)
			}
			if !ctx.HideSpans {
				if row == 0 {
					cells = append(cells, answer)
				} else {
					cells = append(cells, "")
				}
			}
			if !ctx.HideConfidence {
				cells = append(cells, scanConfidence(c.Confidence))
			}
			cells = append(cells, mdEscape(scanSource(c, ctx)))
			bw.line("| " + strings.Join(cells, " | ") + " |")
		}
	}
}

// renderMarkdownAudit writes source-first blockquotes: a source grounding
// several markers appears once, listing every marker it grounds, with its
// excerpt as the quote. Confidence and the answer span honor
// HideConfidence/HideSpans; the source locator honors HideSpans.
func renderMarkdownAudit(bw *markdownWriter, cites []api.Citation, ctx RenderContext) {
	bw.line("#### Citations")

	locations := ctx.citationLocations(cites)
	order, groups := groupCitationsBySource(cites)
	for _, id := range order {
		group := groups[id]
		lead := group[0]

		bw.blank()
		header := "**" + mdEscape(shortSourceID(id)) + "**"
		if title := ctx.citationSourceTitle(lead); title != "" {
			header += " — " + mdEscape(collapseWhitespace(title))
		} else if ctx.citationSourceRemoved(lead) {
			header += " — _(title unavailable)_"
		}
		if !ctx.HideSpans {
			if loc := auditLocator(lead, locations); loc != "" {
				header += " · " + mdEscape(loc)
			}
		}
		bw.line(header)

		bw.line("grounds " + auditGrounds(group, ctx))

		if excerpt := clipExcerpt(lead.Excerpt, ctx.ExcerptBudget); excerpt != "" {
			bw.blank()
			writeBlockquote(bw, excerpt)
		}
	}
}

// writeBlockquote emits a possibly-multi-line excerpt as a markdown blockquote,
// prefixing every line with "> " so a cited passage that spans lines (code,
// config, a table) stays a single quote block instead of the second line
// escaping the quote. A blank interior line becomes a bare ">" to keep the
// block contiguous.
func writeBlockquote(bw *markdownWriter, s string) {
	for line := range strings.SplitSeq(s, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			bw.line(">")
			continue
		}
		bw.line("> " + mdEscapeInline(line))
	}
}

// auditGrounds renders the "[1] (p=0.87, answer 115–241) [3] (…)" tail: one
// clause per marker the source grounds, honoring HideConfidence/HideSpans.
func auditGrounds(group []api.Citation, ctx RenderContext) string {
	var b strings.Builder
	for i, c := range group {
		if i > 0 {
			b.WriteString(" ")
		}
		fmt.Fprintf(&b, "[%d]", c.SourceIndex)
		var parts []string
		if !ctx.HideConfidence {
			if c.Confidence > 0 {
				parts = append(parts, fmt.Sprintf("p=%.2f", c.Confidence))
			}
		}
		if !ctx.HideSpans {
			if span := spanRange(c.StartChar, c.EndChar); span != "" {
				parts = append(parts, "answer "+span)
			}
		}
		if len(parts) > 0 {
			fmt.Fprintf(&b, " (%s)", strings.Join(parts, ", "))
		}
	}
	return b.String()
}

// auditLocator returns the source-document locator for the audit header: the
// resolved "file:line:col" from ctx.citationLocations when available, else the
// raw "src N–M" from SourceStart/SourceEnd, else "".
func auditLocator(c api.Citation, locations map[citationKey]string) string {
	if loc, ok := locations[keyFor(c)]; ok && loc != "" {
		return loc
	}
	if span := spanRange(c.SourceStart, c.SourceEnd); span != "" {
		return "src " + span
	}
	return ""
}

// groupCitationsBySource buckets citations by SourceID, preserving first-seen
// order, so a source grounding several markers appears once. The mirror of
// groupCitationsByIndex for the source-first audit view.
func groupCitationsBySource(cites []api.Citation) ([]string, map[string][]api.Citation) {
	var order []string
	groups := map[string][]api.Citation{}
	for _, c := range cites {
		if _, ok := groups[c.SourceID]; !ok {
			order = append(order, c.SourceID)
		}
		groups[c.SourceID] = append(groups[c.SourceID], c)
	}
	return order, groups
}

// scanSource formats the table's source cell: "id8 title", degrading to
// whichever of the handle and title exists. A source with no resolvable title
// whose ID is absent from the notebook's source list is labeled "(untitled)"
// so a blank cell reads as an unresolved title rather than missing data — not
// "removed", since a citation handle is a granular chunk ID that misses the
// source list even when the source is present.
func scanSource(c api.Citation, ctx RenderContext) string {
	handle := shortSourceID(c.SourceID)
	title := collapseWhitespace(ctx.citationSourceTitle(c))
	if title == "" && ctx.citationSourceRemoved(c) {
		if handle != "" {
			return handle + " (untitled)"
		}
		return "(untitled)"
	}
	switch {
	case handle != "" && title != "":
		return handle + " " + title
	case handle != "":
		return handle
	default:
		return title
	}
}

// scanConfidence renders a source's score for the table's p column as "0.91",
// or "" when unknown (0). No "p=" prefix: the column header carries the label.
func scanConfidence(conf float64) string {
	if conf <= 0 {
		return ""
	}
	return fmt.Sprintf("%.2f", conf)
}

// spanRange renders a character range as "115–241" (en-dash, U+2013) or "409"
// for a point, matching the spec's markdown surface. Returns "" for no real
// span: negative, inverted, or the (0,0) "no metadata" sentinel.
func spanRange(start, end int) string {
	if start < 0 || end < start || (start == 0 && end == 0) {
		return ""
	}
	if end == start {
		return fmt.Sprintf("%d", start)
	}
	return fmt.Sprintf("%d–%d", start, end)
}

// mdEscape escapes the characters that would break markdown table cells or
// inline text: the pipe (cell separator) and backtick (code span). Newlines are
// collapsed to spaces so a value never spills across a row.
func mdEscape(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	return mdEscapeInline(s)
}

// mdEscapeInline escapes the backtick and pipe within a single line, leaving
// all other characters (including whitespace and indentation) verbatim. Used
// for blockquote body lines, where a cited passage is often code whose layout
// must survive but a stray backtick would otherwise open a runaway code span.
func mdEscapeInline(s string) string {
	s = strings.ReplaceAll(s, "`", "\\`")
	s = strings.ReplaceAll(s, "|", "\\|")
	return s
}

// markdownWriter accumulates the first write error so renderChatMarkdown can
// stay flat: callers write freely and check bw.err once at the end.
type markdownWriter struct {
	w   io.Writer
	err error
}

func (bw *markdownWriter) line(s string) {
	if bw.err != nil {
		return
	}
	_, bw.err = fmt.Fprintln(bw.w, s)
}

func (bw *markdownWriter) linef(format string, args ...any) {
	bw.line(fmt.Sprintf(format, args...))
}

func (bw *markdownWriter) blank() {
	bw.line("")
}
