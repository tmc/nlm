package richrender

import (
	"fmt"
	"strings"

	"github.com/tmc/nlm/notebooklm"
)

// weakConfidence is the grounding-score threshold below which a citation reads
// as weakly supported and its confidence renders amber. It is a reading signal,
// not a filter: weak citations still print.
const weakConfidence = 0.75

// groupCitationsByIndex buckets citations by SourceIndex, preserving the order
// indices first appear in the stream.
func groupCitationsByIndex(cites []notebooklm.Citation) ([]int, map[int][]notebooklm.Citation) {
	var order []int
	groups := map[int][]notebooklm.Citation{}
	for _, c := range cites {
		if _, ok := groups[c.SourceIndex]; !ok {
			order = append(order, c.SourceIndex)
		}
		groups[c.SourceIndex] = append(groups[c.SourceIndex], c)
	}
	return order, groups
}

// formatAnswerSpan renders a marker's answer-text range as "answer 42-205", or
// "answer 409" for a single non-zero point. The "answer" label is load-bearing:
// this offset indexes the answer, never the source (see formatSourceSpan).
// Returns "" for no real span (negative, inverted, or the (0,0) "no metadata"
// sentinel — never "answer 0").
func formatAnswerSpan(start, end int) string {
	return formatLabeledSpan("answer", start, end)
}

// formatSourceSpan renders a citation's source-document range as
// "src 965670-966914" — where the excerpt lives inside the source, from
// SourceStart/SourceEnd. Distinct from the answer span; same empty-span rules.
func formatSourceSpan(start, end int) string {
	return formatLabeledSpan("src", start, end)
}

// formatLabeledSpan renders "<label> N-M" (or "<label> N" for a point), the
// shared shape behind the answer/source span formatters. Returns "" when there
// is no real span: a negative range, an inverted range, or the zero value
// (0,0), which is the "no span metadata" sentinel and must not render as "N 0".
func formatLabeledSpan(label string, start, end int) string {
	if start < 0 || end < start || (start == 0 && end == 0) {
		return ""
	}
	if end == start {
		return fmt.Sprintf("%s %d", label, start)
	}
	return fmt.Sprintf("%s %d-%d", label, start, end)
}

// shortSourceID returns the 8-char prefix of a source ID (a UUID), or the
// whole ID when it is already 8 chars or shorter. Empty in, empty out.
func shortSourceID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}

// truncateExcerpt collapses whitespace and clips to max runes with an ellipsis.
// Use it for single-line surfaces (the TUI citation rows, where a row must not
// wrap or break alignment). Surfaces that can display structure — HTML's
// pre-wrap excerpt box, a Markdown blockquote — must use clipExcerpt instead so
// cited code, config, and tables keep their line and indent structure.
func truncateExcerpt(s string, max int) string {
	return clipRunes(collapseWhitespace(s), max)
}

// clipExcerpt clips to max runes with an ellipsis but preserves internal
// whitespace (newlines, tabs, indentation). Cited passages are frequently code
// or config whose meaning lives in their layout; the multi-line surfaces render
// that structure, so flattening it here would corrupt the evidence a citation
// exists to show. Leading/trailing whitespace is trimmed so the excerpt does not
// open on a blank line.
func clipExcerpt(s string, max int) string {
	return clipRunes(strings.TrimSpace(s), max)
}

// clipRunes truncates s to max runes, appending an ellipsis when it clips.
func clipRunes(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "…"
}

func collapseWhitespace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
