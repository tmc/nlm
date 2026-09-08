package richrender

import "strings"

// markdownTableHeader recognizes a header followed by a delimiter row. Requiring
// both prevents ordinary prose containing pipes from becoming a table.
func markdownTableHeader(lines []string, i int) ([]string, []string) {
	if i+1 >= len(lines) {
		return nil, nil
	}
	header := markdownTableRow(lines[i])
	delimiters := markdownTableRow(lines[i+1])
	if len(header) == 0 || len(header) != len(delimiters) {
		return nil, nil
	}
	alignment := make([]string, len(header))
	for j, cell := range delimiters {
		if dashes := strings.TrimSuffix(strings.TrimPrefix(cell, ":"), ":"); len(dashes) < 3 || strings.Trim(dashes, "-") != "" {
			return nil, nil
		}
		switch {
		case strings.HasPrefix(cell, ":") && strings.HasSuffix(cell, ":"):
			alignment[j] = "align-center"
		case strings.HasSuffix(cell, ":"):
			alignment[j] = "align-right"
		default:
			alignment[j] = "align-left"
		}
	}
	return header, alignment
}

func markdownTableRow(line string) []string {
	line = strings.TrimSpace(line)
	var cells []string
	start := 0
	for i := 0; i < len(line); i++ {
		if line[i] == '\\' {
			i++ // An escaped pipe belongs to the cell, even inside code.
		} else if line[i] == '|' {
			cells = append(cells, strings.TrimSpace(line[start:i]))
			start = i + 1
		}
	}
	if len(cells) == 0 {
		return nil
	}
	cells = append(cells, strings.TrimSpace(line[start:]))
	if strings.HasPrefix(line, "|") {
		cells = cells[1:]
	}
	if start == len(line) {
		cells = cells[:len(cells)-1]
	}
	return cells
}

func markdownTableNodes(block markdownSubsetBlock, msgIdx int, markers map[int]htmlMarker) answerNode {
	table := answerNode{Tag: "table"}
	for i, row := range block.rows {
		tag, group := "td", "tbody"
		if i == 0 {
			tag, group = "th", "thead"
		}
		tr := answerNode{Tag: "tr"}
		for j, cell := range row {
			tr.Children = append(tr.Children, answerNode{
				Tag: tag, Class: block.alignment[j],
				Children: unescapeTablePipes(plainMarkdownInlineNodes(cell, msgIdx, markers)),
			})
		}
		if i <= 1 {
			table.Children = append(table.Children, answerNode{Tag: group})
		}
		last := len(table.Children) - 1
		table.Children[last].Children = append(table.Children[last].Children, tr)
	}
	return answerNode{Tag: "div", Class: "table-scroll", Children: []answerNode{table}}
}

// Keep the pieces separate so grounding can locate them in the original text,
// where the backslash is still present. Inline formatting remains on its parent.
func unescapeTablePipes(nodes []answerNode) []answerNode {
	var out []answerNode
	for _, node := range nodes {
		if len(node.Children) > 0 {
			node.Children = unescapeTablePipes(node.Children)
		}
		parts := strings.Split(node.Text, `\|`)
		if len(parts) == 1 {
			out = append(out, node)
			continue
		}
		var children []answerNode
		for i, part := range parts {
			if i > 0 {
				children = append(children, answerNode{Text: "|"})
			}
			if part != "" {
				children = append(children, answerNode{Text: part})
			}
		}
		if node.Tag == "" {
			out = append(out, children...)
		} else {
			node.Text, node.Children = "", children
			out = append(out, node)
		}
	}
	return out
}

func hasMarkdownTable(content string) bool {
	if !strings.Contains(content, "|") {
		return false
	}
	lines := strings.Split(content, "\n")
	for i := range lines {
		if header, _ := markdownTableHeader(lines, i); header != nil {
			return true
		}
	}
	return false
}
