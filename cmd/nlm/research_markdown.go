package main

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/tmc/nlm/notebooklm"
)

var researchCitationPattern = regexp.MustCompile(`(?i)\[cite:\s*([0-9]+(?:\s*,\s*[0-9]+)*)\]`)

// researchMarkdown replaces NotebookLM's [cite: 1, 2] markers with Markdown
// footnote references and appends definitions for the cited research sources.
func researchMarkdown(report string, sources []notebooklm.ResearchSource) string {
	byIndex := make(map[int]notebooklm.ResearchSource)
	for _, source := range sources {
		if source.CitationIndex <= 0 || strings.TrimSpace(source.URL) == "" {
			continue
		}
		if _, ok := byIndex[source.CitationIndex]; !ok {
			byIndex[source.CitationIndex] = source
		}
	}
	if len(byIndex) == 0 {
		return report
	}

	used := make(map[int]bool)
	var out strings.Builder
	fence := ""
	for _, line := range strings.SplitAfter(report, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case fence == "" && strings.HasPrefix(trimmed, "```"):
			fence = "```"
			out.WriteString(line)
			continue
		case fence == "" && strings.HasPrefix(trimmed, "~~~"):
			fence = "~~~"
			out.WriteString(line)
			continue
		case fence != "":
			out.WriteString(line)
			if strings.HasPrefix(trimmed, fence) {
				fence = ""
			}
			continue
		}
		out.WriteString(researchCitationPattern.ReplaceAllStringFunc(line, func(marker string) string {
			match := researchCitationPattern.FindStringSubmatch(marker)
			if len(match) != 2 {
				return marker
			}
			var refs strings.Builder
			var unknown []string
			seen := make(map[int]bool)
			for _, field := range strings.Split(match[1], ",") {
				n, err := strconv.Atoi(strings.TrimSpace(field))
				if err != nil || seen[n] {
					continue
				}
				seen[n] = true
				if _, ok := byIndex[n]; !ok {
					unknown = append(unknown, strconv.Itoa(n))
					continue
				}
				fmt.Fprintf(&refs, "[^%d]", n)
				used[n] = true
			}
			if refs.Len() == 0 {
				return marker
			}
			if len(unknown) != 0 {
				fmt.Fprintf(&refs, "[cite: %s]", strings.Join(unknown, ", "))
			}
			return refs.String()
		}))
	}
	if len(used) == 0 {
		return report
	}

	text := out.String()
	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	if !strings.HasSuffix(text, "\n\n") {
		text += "\n"
	}
	indexes := make([]int, 0, len(used))
	for index := range used {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)
	var definitions strings.Builder
	for _, index := range indexes {
		source := byIndex[index]
		title := strings.Join(strings.Fields(source.Title), " ")
		if title == "" {
			title = fmt.Sprintf("Source %d", index)
		}
		title = strings.NewReplacer(`\`, `\\`, `]`, `\]`).Replace(title)
		fmt.Fprintf(&definitions, "[^%d]: [%s](<%s>)\n", index, title, strings.TrimSpace(source.URL))
	}
	return text + definitions.String()
}
