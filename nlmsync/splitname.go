package nlmsync

import (
	"strings"
)

// A split part keeps its chunk number and adds one letter per split level in
// a group of its own: the halves of "name (pt3)" are "name (pt3) (a)" and
// "name (pt3) (b)", and the halves of those are "name (pt3) (aa)" and
// "name (pt3) (ab)". The first chunk carries no number of its own, so its
// halves are "name (pt1) (a)" and "name (pt1) (b)".
//
// One letter group replaces the repeated " (split1) (split2)" chain, so even
// a deep tree stays short and readable, and the parent chain is still
// recoverable from the title alone — a later run rebuilds the split layout
// without a local manifest.
const (
	ptPrefix   = " (pt"
	firstChunk = " (pt1)"
)

// splitChildName names the i-th (1-based) half of name.
func splitChildName(name string, i int) string {
	base, chunk, letters := splitTitle(name)
	if chunk == "" {
		chunk = firstChunk
	}
	return base + chunk + " (" + letters + string(rune('a'+i-1)) + ")"
}

// splitTitle decomposes a part title into the family base, its chunk suffix
// (" (ptN)", empty for the bare first chunk) and its split letters (empty
// for a part that has not been split).
func splitTitle(title string) (base, chunk, letters string) {
	base = title
	if group, rest, ok := trailingGroup(base); ok && isLetters(group) {
		// Letters are a split path only behind a chunk number: sync writes
		// them that way, and a family name may legitimately end in a word
		// group of its own ("notes (draft)").
		if inner, outer, ok := trailingGroup(rest); ok && isChunkGroup(inner) {
			return outer, " (" + inner + ")", group
		}
		return title, "", ""
	}
	if group, rest, ok := trailingGroup(base); ok && isChunkGroup(group) {
		return rest, " (" + group + ")", ""
	}
	return title, "", ""
}

// isChunkGroup reports whether a "(...)" group is a chunk number, "pt3".
func isChunkGroup(group string) bool {
	return strings.HasPrefix(group, "pt") && isDigits(group[2:])
}

// trailingGroup splits a trailing "(...)" group off title.
func trailingGroup(title string) (group, rest string, ok bool) {
	if !strings.HasSuffix(title, ")") {
		return "", title, false
	}
	idx := strings.LastIndex(title, " (")
	if idx < 0 {
		return "", title, false
	}
	return title[idx+2 : len(title)-1], title[:idx], true
}

func isLetters(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < 'a' || c > 'z' {
			return false
		}
	}
	return true
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// trimSplitSuffix removes one split level from title, in either the current
// letter-group form or the legacy one-suffix-per-level " (split1)" form. It
// reports whether anything was trimmed. Dropping the last letter off the
// first chunk yields the bare family name, since that chunk carries no
// number of its own.
func trimSplitSuffix(title string) (string, bool) {
	base, chunk, letters := splitTitle(title)
	if letters == "" {
		if strings.HasSuffix(title, " (split1)") || strings.HasSuffix(title, " (split2)") {
			return title[:len(title)-len(" (split1)")], true
		}
		return title, false
	}
	if letters = letters[:len(letters)-1]; letters != "" {
		return base + chunk + " (" + letters + ")", true
	}
	if chunk == firstChunk {
		return base, true
	}
	return base + chunk, true
}
