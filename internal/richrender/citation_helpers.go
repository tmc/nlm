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
	s = decodeNumberedExcerpt(s)
	s = formatFlattenedExcerptTable(s)
	return clipRunes(strings.TrimSpace(s), max)
}

// decodeNumberedExcerpt decodes the escaped line separators emitted by
// JSONL-style source exports. Requiring repeated "\n<line>\t" records avoids
// changing ordinary source text that merely mentions \n or \t.
func decodeNumberedExcerpt(s string) string {
	if numberedEscapeCount(s) < 2 {
		return s
	}
	return strings.NewReplacer(`\n`, "\n", `\t`, "\t", `\r`, "\r").Replace(s)
}

func numberedEscapeCount(s string) int {
	count := 0
	for start := 0; ; {
		i := strings.Index(s[start:], `\n`)
		if i < 0 {
			return count
		}
		i += start + len(`\n`)
		j := i
		for j < len(s) && s[j] >= '0' && s[j] <= '9' {
			j++
		}
		if j > i && strings.HasPrefix(s[j:], `\t`) {
			count++
		}
		start = i
	}
}

// formatFlattenedExcerptTable restores boundaries in legacy saved excerpts
// produced before table cells were joined with tabs and newlines. A repeated
// dotted row prefix (for example "siphon.") is strong evidence of a flattened
// two-column table; ordinary prose and isolated identifiers are unchanged.
func formatFlattenedExcerptTable(s string) string {
	prefix := repeatedDottedPrefix(s)
	if prefix == "" {
		return s
	}

	var b strings.Builder
	endsNewline := false
	for rest := s; ; {
		i := strings.Index(rest, prefix)
		if i < 0 {
			b.WriteString(rest)
			break
		}
		head := rest[:i]
		if b.Len() == 0 {
			head = splitLowerUpper(head, " ")
			b.WriteString(head)
		} else {
			b.WriteString(head)
		}
		if head != "" {
			endsNewline = head[len(head)-1] == '\n'
		}
		if b.Len() > 0 && !endsNewline {
			b.WriteByte('\n')
		}
		b.WriteString(prefix)
		endsNewline = false
		rest = rest[i+len(prefix):]

		j := 0
		for j < len(rest) && (rest[j] >= 'a' && rest[j] <= 'z' ||
			rest[j] >= '0' && rest[j] <= '9' || rest[j] == '_' || rest[j] == '-') {
			j++
		}
		b.WriteString(rest[:j])
		if j > 0 && j < len(rest) && rest[j] >= 'A' && rest[j] <= 'Z' {
			b.WriteByte('\t')
		}
		rest = rest[j:]
	}
	return b.String()
}

func repeatedDottedPrefix(s string) string {
	counts := make(map[string]int)
	for i := 0; i < len(s); i++ {
		if s[i] != '.' {
			continue
		}
		start := i
		for start > 0 && (s[start-1] >= 'a' && s[start-1] <= 'z' ||
			s[start-1] >= 'A' && s[start-1] <= 'Z' ||
			s[start-1] >= '0' && s[start-1] <= '9' ||
			s[start-1] == '_' || s[start-1] == '-') {
			start--
		}
		if start == i {
			continue
		}
		for suffix := start; suffix < i; suffix++ {
			// Skip non-letters. Parenthesized for clarity: the middle term is the
			// gap between 'Z' and 'a'. Equivalent to !unicode-letter over ASCII.
			c := s[suffix]
			if c < 'A' || (c > 'Z' && c < 'a') || c > 'z' {
				continue
			}
			counts[s[suffix:i+1]]++
		}
	}
	best := ""
	for prefix, count := range counts {
		if count >= 3 && len(prefix) > len(best) {
			best = prefix
		}
	}
	joined := 0
	for start := 0; best != ""; {
		i := strings.Index(s[start:], best)
		if i < 0 {
			break
		}
		i += start
		if i > 0 && s[i-1] >= 'a' && s[i-1] <= 'z' {
			joined++
		}
		start = i + len(best)
	}
	if joined < 2 {
		return ""
	}
	return best
}

func splitLowerUpper(s, separator string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if i > 0 && s[i-1] >= 'a' && s[i-1] <= 'z' && s[i] >= 'A' && s[i] <= 'Z' {
			b.WriteString(separator)
		}
		b.WriteByte(s[i])
	}
	return b.String()
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
