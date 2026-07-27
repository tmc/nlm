package richrender

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

var (
	trailingMathCitationRE = regexp.MustCompile(`(?s)^(.*?)(\\quad[ \t]*)(\[[0-9][0-9,\s-]*\])[ \t]*$`)
	latexTokenRE           = regexp.MustCompile(`\\[A-Za-z]+|[\^_]`)
)

const maxInlineCitationMathRunes = 512

type mathCitationLift struct {
	math      string
	inner     string
	liftStart int
	close     string
}

// liftTrailingMathCitation removes a trailing "\quad [N]" citation from one
// complete TeX token. It accepts compound markers and requires actual LaTeX
// syntax in the body, so unrelated dollar-delimited prose is left unchanged.
func liftTrailingMathCitation(token string) (mathCitationLift, bool) {
	open, close := "$", "$"
	if strings.HasPrefix(token, "$$") {
		open, close = "$$", "$$"
	}
	if !strings.HasPrefix(token, open) || !strings.HasSuffix(token, close) ||
		len(token) <= len(open)+len(close) {
		return mathCitationLift{}, false
	}
	if open == "$" && utf8.RuneCountInString(token) > maxInlineCitationMathRunes {
		return mathCitationLift{}, false
	}

	body := token[len(open) : len(token)-len(close)]
	match := trailingMathCitationRE.FindStringSubmatchIndex(body)
	if match == nil || !latexTokenRE.MatchString(body[:match[4]]) {
		return mathCitationLift{}, false
	}
	inner := body[match[6]+1 : match[7]-1]
	liftStart := match[4]
	for liftStart > 0 && (body[liftStart-1] == ' ' || body[liftStart-1] == '\t') {
		liftStart--
	}
	mathBody := strings.TrimRight(body[:liftStart], " \t")
	return mathCitationLift{
		math:      open + mathBody + close,
		inner:     inner,
		liftStart: len(open) + liftStart,
		close:     close,
	}, true
}

// mathCitationAt finds the complete math token containing marker [start,end)
// and returns the byte range occupied by "\quad [N]$". The caller can replace
// that range with the closing delimiter followed by a normal citation link.
func mathCitationAt(text string, start, end int) (tokenStart, from, to int, lift mathCitationLift, ok bool) {
	if start < 0 || end > len(text) || start >= end {
		return 0, 0, 0, mathCitationLift{}, false
	}

	after := end
	for after < len(text) && (text[after] == ' ' || text[after] == '\t') {
		after++
	}
	close := "$"
	if strings.HasPrefix(text[after:], "$$") {
		close = "$$"
	} else if !strings.HasPrefix(text[after:], "$") {
		return 0, 0, 0, mathCitationLift{}, false
	}
	tokenEnd := after + len(close)

	before := text[:start]
	if close == "$$" {
		tokenStart = strings.LastIndex(before, "$$")
	} else {
		tokenStart = strings.LastIndex(before, "$")
		if tokenStart > 0 && text[tokenStart-1] == '$' {
			return 0, 0, 0, mathCitationLift{}, false
		}
	}
	if tokenStart < 0 {
		return 0, 0, 0, mathCitationLift{}, false
	}

	lift, ok = liftTrailingMathCitation(text[tokenStart:tokenEnd])
	if !ok || lift.close != close {
		return 0, 0, 0, mathCitationLift{}, false
	}
	return tokenStart, tokenStart + lift.liftStart, tokenEnd, lift, true
}

type mathTextRef struct {
	node       *answerNode
	start, end int
}

type splitMathCitation struct {
	start, end int
	lift       mathCitationLift
}

// liftSplitMathCitations repairs math tokens split across rich-tree runs. The
// wire offsets may put the delimiters, grounded text, and marker in separate
// nodes; rebuilding the clean token in one escaped text node gives MathJax a
// complete delimiter pair and leaves the citation link outside it.
func liftSplitMathCitations(nodes []answerNode, msgIdx int, byIndex map[int]htmlMarker) []answerNode {
	for {
		var refs []mathTextRef
		var text strings.Builder
		collectMathTextRefs(nodes, false, &refs, &text)
		stream := text.String()

		var edit splitMathCitation
		found := false
		locs := htmlMarkerRe.FindAllStringSubmatchIndex(stream, -1)
		for i := len(locs) - 1; i >= 0; i-- {
			loc := locs[i]
			tokenStart, _, tokenEnd, lift, ok := mathCitationAt(stream, loc[0], loc[1])
			if !ok || sameMathTextRef(refs, tokenStart, tokenEnd) {
				continue
			}
			edit = splitMathCitation{start: tokenStart, end: tokenEnd, lift: lift}
			found = true
			break
		}
		if !found {
			return nodes
		}
		if !applySplitMathCitation(refs, edit, msgIdx, byIndex) {
			return nodes
		}
	}
}

func collectMathTextRefs(nodes []answerNode, inCode bool, refs *[]mathTextRef, text *strings.Builder) {
	for i := range nodes {
		node := &nodes[i]
		code := inCode || node.Tag == "code" || node.Tag == "pre"
		if node.Text != "" && !code && node.Tag != "a" {
			start := text.Len()
			text.WriteString(node.Text)
			*refs = append(*refs, mathTextRef{node: node, start: start, end: text.Len()})
		}
		if len(node.Children) > 0 {
			collectMathTextRefs(node.Children, code, refs, text)
		}
	}
}

func sameMathTextRef(refs []mathTextRef, start, end int) bool {
	for _, ref := range refs {
		if start >= ref.start && end <= ref.end {
			return true
		}
	}
	return false
}

func applySplitMathCitation(refs []mathTextRef, edit splitMathCitation, msgIdx int, byIndex map[int]htmlMarker) bool {
	startRef := -1
	var grounded *answerNode
	for i, ref := range refs {
		if ref.end <= edit.start || ref.start >= edit.end {
			continue
		}
		if startRef < 0 && edit.start >= ref.start && edit.start < ref.end {
			startRef = i
		}
		if grounded == nil && ref.node.Class == "grounded" {
			copy := *ref.node
			copy.Children = nil
			grounded = &copy
		}
	}
	if startRef < 0 {
		return false
	}

	start := refs[startRef]
	prefixEnd := edit.start - start.start
	prefix := start.node.Text[:prefixEnd]
	suffix := ""
	if edit.end <= start.end {
		suffix = start.node.Text[edit.end-start.start:]
	}
	for i, ref := range refs {
		if i == startRef || ref.end <= edit.start || ref.start >= edit.end {
			continue
		}
		lo, hi := 0, len(ref.node.Text)
		if edit.start > ref.start {
			lo = edit.start - ref.start
		}
		if edit.end < ref.end {
			hi = edit.end - ref.start
		}
		ref.node.Text = ref.node.Text[:lo] + ref.node.Text[hi:]
		if ref.node.Text == "" && len(ref.node.Children) == 0 {
			*ref.node = answerNode{}
		}
	}

	math := answerNode{Text: edit.lift.math}
	if grounded != nil {
		grounded.Text = edit.lift.math
		math = *grounded
	}
	children := make([]answerNode, 0, 3)
	if prefix != "" {
		children = append(children, answerNode{Text: prefix})
	}
	children = append(children, liftedMathCitationNodes(math, edit.lift, msgIdx, byIndex)...)
	if suffix != "" {
		children = append(children, answerNode{Text: suffix})
	}
	*start.node = answerNode{Tag: "span", Children: children}
	return true
}

// liftedMathCitationNodes keeps an inline citation beside its math token. For
// display math it groups the equation and marker in one layout row so CSS can
// center the equation while placing the citation in the right gutter.
func liftedMathCitationNodes(math answerNode, lift mathCitationLift, msgIdx int, byIndex map[int]htmlMarker) []answerNode {
	markers := markerNodes(msgIdx, lift.inner, byIndex)
	if lift.close != "$$" {
		return append([]answerNode{math}, markers...)
	}
	return []answerNode{{
		Tag:   "span",
		Class: "math-display-row",
		Children: []answerNode{
			{Tag: "span", Class: "math-display-equation", Children: []answerNode{math}},
			{Tag: "span", Class: "math-display-cite", Children: markers},
		},
	}}
}
