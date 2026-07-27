package richrender

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

func renderNoteMarkdown(out io.Writer, doc NoteDocument) error {
	bw := &markdownWriter{w: out}
	bw.line("# " + doc.Title)
	bw.blank()
	if doc.Flat != "" {
		bw.line(markdownMath(doc.Flat))
	} else {
		occurrences := noteMarkerOccurrences(doc.Citations)
		renderRichNoteMarkdown(bw, projectRichDocument(doc.Rich), occurrences)
	}
	if len(doc.Citations) > 0 {
		bw.blank()
		renderMarkdownScan(bw, doc.Citations, RenderContext{})
	}
	return bw.err
}

func renderNoteText(out io.Writer, doc NoteDocument) error {
	if _, err := fmt.Fprintf(out, "# %s\n\n", doc.Title); err != nil {
		return err
	}
	body := doc.Flat
	if body == "" && doc.Rich != nil {
		var buf strings.Builder
		bw := &markdownWriter{w: &buf}
		renderRichNoteMarkdown(bw, projectRichDocument(doc.Rich), noteMarkerOccurrences(doc.Citations))
		if bw.err != nil {
			return bw.err
		}
		body = strings.TrimSuffix(buf.String(), "\n")
	}
	renderPersistedAssistant(out, out, storedMessage{
		Role:      "assistant",
		Content:   body,
		Citations: doc.Citations,
	}, citationModeList, persistedRenderConfig{})
	return nil
}

func renderRichNoteMarkdown(bw *markdownWriter, blocks []richBlockOut, occurrences []noteMarkerOccurrence) {
	for i, block := range blocks {
		if i > 0 && block.Kind != blockSeparator {
			bw.blank()
		}
		switch block.Kind {
		case blockSeparator:
			bw.blank()
			bw.line("---")
		case blockHidden:
			continue
		case blockList:
			for _, item := range block.Items {
				bw.line(strings.Repeat("  ", item.Nesting) + "- " + markdownNoteRuns(item.Runs, occurrences))
			}
		case blockCodeBlock:
			bw.line("```")
			bw.line(runsText(block.Runs))
			bw.line("```")
		default:
			prefix := ""
			if block.Kind == blockParagraph && anyHeadingRun(block.Runs) {
				prefix = "## "
			}
			if text := markdownNoteRuns(block.Runs, occurrences); text != "" {
				bw.line(prefix + text)
			}
		}
	}
}

func markdownNoteRuns(runs []richRun, occurrences []noteMarkerOccurrence) string {
	var out strings.Builder
	for _, run := range runs {
		runText := noteRunTextWithMarkers(run, occurrences)
		text := markdownMath(runText)
		switch {
		case run.Code:
			text = "`" + strings.ReplaceAll(runText, "`", "\\`") + "`"
		case run.Link != "" && safeNoteLink(run.Link):
			text = "[" + text + "](" + run.Link + ")"
		case run.Emphasis:
			text = "*" + text + "*"
		}
		out.WriteString(text)
	}
	return out.String()
}

func noteRunTextWithMarkers(run richRun, occurrences []noteMarkerOccurrence) string {
	type placement struct {
		at    int
		index int
	}
	var placements []placement
	seen := make(map[string]bool)
	u16 := newUTF16RuneMap(run.Text)
	for _, occurrence := range occurrences {
		if occurrence.End <= run.Start || occurrence.End > run.End {
			continue
		}
		at := u16.rune(occurrence.End - run.Start)
		key := fmt.Sprintf("%d:%d", at, occurrence.Index)
		if seen[key] {
			continue
		}
		seen[key] = true
		placements = append(placements, placement{at: at, index: occurrence.Index})
	}
	sort.SliceStable(placements, func(i, j int) bool { return placements[i].at < placements[j].at })

	runes := []rune(run.Text)
	var out strings.Builder
	at := 0
	for _, placement := range placements {
		if placement.at < at || placement.at > len(runes) {
			continue
		}
		out.WriteString(string(runes[at:placement.at]))
		fmt.Fprintf(&out, "[%d]", placement.index)
		at = placement.at
	}
	out.WriteString(string(runes[at:]))
	return out.String()
}

func markdownMath(text string) string {
	return noteMathRE.ReplaceAllStringFunc(text, func(math string) string {
		return "`" + strings.ReplaceAll(math, "`", "\\`") + "`"
	})
}

func safeNoteLink(link string) bool {
	return strings.HasPrefix(link, "https://") || strings.HasPrefix(link, "http://") || strings.HasPrefix(link, "#")
}
