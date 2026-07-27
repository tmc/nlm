package main

import (
	"regexp"
	"strings"
)

var (
	markdownInlineRE   = regexp.MustCompile(`\*\*[^*\n]+\*\*|\*[^*\n]+\*|` + "`[^`\n]+`" + `|\$\$[^$]+\$\$|\$[^$\n]+\$|\[[0-9][0-9,\s-]*\]`)
	markdownHeadingRE  = regexp.MustCompile(`^\s*(#{1,6})\s+`)
	markdownListRE     = regexp.MustCompile(`^(\s*)([-+*]|\d+\.)\s+`)
	markdownSignalRE   = regexp.MustCompile(`(?m)^\s*(?:#{1,6}\s+|[-+*]\s+|\d+\.\s+|---\s*$|` + "```" + `)|\*\*[^*\n]+\*\*|\*[^*\n]+\*|` + "`[^`\n]+`")
	trailingFollowUpRE = regexp.MustCompile(`(?s)\n\n(?:---\s*\n\n)?(?:[\p{So}\p{Sk}]\s*)?(?:(?i:\*\*Next Steps?\*\*|Next Steps?)\s*:\s*)?Would you like\b[^?]*\?\s*$`)
)

type markdownSubsetOptions struct {
	headingLevels bool
	orderedLists  bool
	msgIdx        int
}

type markdownSubsetBlock struct {
	kind         blockKind
	headingLevel int
	ordered      bool
	text         string
	items        []string
	nestings     []int
}

func withoutChatFollowUps(doc chatDocument) chatDocument {
	out := doc
	out.Messages = append([]chatDocMessage(nil), doc.Messages...)
	for i := range out.Messages {
		m := &out.Messages[i]
		if m.Role == "assistant" && m.Rich == nil {
			m.Content = trailingFollowUpRE.ReplaceAllString(m.Content, "")
		}
	}
	return out
}

func hasMarkdownSubset(content string) bool {
	return !looksLikeJSON(content) && markdownSignalRE.MatchString(content)
}

func richMarkdownOverlayNodes(projected []richBlockOut, markdown string, byIndex map[int]htmlMarker) []answerNode {
	return markdownSubsetOverlayNodes(projected, markdown, byIndex, markdownSubsetOptions{})
}

func markdownSubsetNodes(markdown string, byIndex map[int]htmlMarker) []answerNode {
	return markdownSubsetOverlayNodes([]richBlockOut{{Kind: blockParagraph}}, markdown, byIndex, markdownSubsetOptions{})
}

func chatMarkdownSubsetNodes(msgIdx int, markdown string, byIndex map[int]htmlMarker) []answerNode {
	return markdownSubsetOverlayNodes(nil, markdown, byIndex, markdownSubsetOptions{
		headingLevels: true,
		orderedLists:  true,
		msgIdx:        msgIdx,
	})
}

func markdownSubsetOverlayNodes(projected []richBlockOut, markdown string, byIndex map[int]htmlMarker, opts markdownSubsetOptions) []answerNode {
	if markdown == "" {
		return nil
	}
	blocks := parseMarkdownSubsetBlocks(markdown)
	var out []answerNode
	for i, block := range blocks {
		var tree richBlockOut
		if i < len(projected) {
			tree = projected[i]
		}
		switch block.kind {
		case blockSeparator:
			out = append(out, answerNode{Tag: "hr"})
		case blockCodeBlock:
			out = append(out, answerNode{Tag: "pre", Children: []answerNode{{
				Tag: "code", Children: markerTextNodes(block.text, opts.msgIdx, byIndex),
			}}})
		case blockList:
			var items []answerNode
			for j, item := range block.items {
				nesting := 0
				if j < len(block.nestings) {
					nesting = block.nestings[j]
				}
				if tree.Kind == blockList && j < len(tree.Items) {
					nesting = tree.Items[j].Nesting
				}
				items = append(items, answerNode{
					Tag:      "li",
					Class:    nestClass(nesting),
					Children: plainMarkdownInlineNodes(item, opts.msgIdx, byIndex),
				})
			}
			tag := "ul"
			if opts.orderedLists && block.ordered {
				tag = "ol"
			}
			out = append(out, answerNode{Tag: tag, Children: items})
		default:
			tag := "p"
			if block.headingLevel != 0 {
				tag = "h4"
				if opts.headingLevels && block.headingLevel <= 3 {
					tag = "h3"
				}
			}
			out = append(out, answerNode{Tag: tag, Children: plainMarkdownInlineNodes(block.text, opts.msgIdx, byIndex)})
		}
	}
	return out
}

func markerIndicesFromText(text string) []int {
	var out []int
	seen := make(map[int]bool)
	for _, match := range htmlMarkerRe.FindAllStringSubmatch(text, -1) {
		indices, ok := citationIndices(match[1])
		if !ok {
			continue
		}
		for _, index := range indices {
			if !seen[index] {
				seen[index] = true
				out = append(out, index)
			}
		}
	}
	return out
}

func parseMarkdownSubsetBlocks(markdown string) []markdownSubsetBlock {
	var out []markdownSubsetBlock
	var paragraph []string
	var list *markdownSubsetBlock
	flushParagraph := func() {
		if len(paragraph) != 0 {
			out = append(out, markdownSubsetBlock{kind: blockParagraph, text: strings.Join(paragraph, "\n")})
			paragraph = nil
		}
	}
	flushList := func() {
		if list != nil {
			out = append(out, *list)
			list = nil
		}
	}
	lines := strings.Split(strings.ReplaceAll(markdown, "\r\n", "\n"), "\n")
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "```"):
			flushParagraph()
			flushList()
			var code []string
			for i++; i < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[i]), "```"); i++ {
				code = append(code, lines[i])
			}
			out = append(out, markdownSubsetBlock{kind: blockCodeBlock, text: strings.Join(code, "\n")})
		case trimmed == "":
			flushParagraph()
			flushList()
		case trimmed == "---":
			flushParagraph()
			flushList()
			out = append(out, markdownSubsetBlock{kind: blockSeparator})
		case markdownHeadingRE.MatchString(line):
			flushParagraph()
			flushList()
			match := markdownHeadingRE.FindStringSubmatch(line)
			out = append(out, markdownSubsetBlock{
				kind:         blockParagraph,
				headingLevel: len(match[1]),
				text:         markdownHeadingRE.ReplaceAllString(line, ""),
			})
		case markdownListRE.MatchString(line):
			flushParagraph()
			match := markdownListRE.FindStringSubmatch(line)
			ordered := match[2][0] >= '0' && match[2][0] <= '9'
			if list == nil || list.ordered != ordered {
				flushList()
				list = &markdownSubsetBlock{kind: blockList, ordered: ordered}
			}
			list.items = append(list.items, markdownListRE.ReplaceAllString(line, ""))
			list.nestings = append(list.nestings, len(strings.ReplaceAll(match[1], "\t", "  "))/2)
		default:
			flushList()
			paragraph = append(paragraph, line)
		}
	}
	flushParagraph()
	flushList()
	return out
}

func plainMarkdownInlineNodes(text string, msgIdx int, byIndex map[int]htmlMarker) []answerNode {
	var out []answerNode
	at := 0
	for _, match := range markdownInlineRE.FindAllStringIndex(text, -1) {
		if match[0] > at {
			out = append(out, markerTextNodes(text[at:match[0]], msgIdx, byIndex)...)
		}
		token := text[match[0]:match[1]]
		switch {
		case strings.HasPrefix(token, "["):
			out = append(out, markerNodes(msgIdx, token[1:len(token)-1], byIndex)...)
		case strings.HasPrefix(token, "$"):
			out = append(out, noteMathCitationNodes(token, msgIdx, byIndex)...)
		case strings.HasPrefix(token, "**"):
			out = append(out, answerNode{Tag: "strong", Children: markerTextNodes(token[2:len(token)-2], msgIdx, byIndex)})
		case strings.HasPrefix(token, "*"):
			out = append(out, answerNode{Tag: "em", Children: markerTextNodes(token[1:len(token)-1], msgIdx, byIndex)})
		default:
			out = append(out, answerNode{Tag: "code", Children: markerTextNodes(token[1:len(token)-1], msgIdx, byIndex)})
		}
		at = match[1]
	}
	if at < len(text) {
		out = append(out, markerTextNodes(text[at:], msgIdx, byIndex)...)
	}
	return out
}

func markerTextNodes(text string, msgIdx int, byIndex map[int]htmlMarker) []answerNode {
	var out []answerNode
	at := 0
	for _, match := range htmlMarkerRe.FindAllStringSubmatchIndex(text, -1) {
		if match[0] > at {
			out = append(out, answerNode{Text: text[at:match[0]]})
		}
		out = append(out, markerNodes(msgIdx, text[match[2]:match[3]], byIndex)...)
		at = match[1]
	}
	if at < len(text) {
		out = append(out, answerNode{Text: text[at:]})
	}
	return out
}
