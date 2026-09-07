package nlmsync

import "strings"

// partIdentity is relative to a literal family base. Chunk 1 has a bare
// canonical title; split paths always include their chunk number.
type partIdentity struct {
	chunk string
	path  string
}

func (p partIdentity) title(base string) string {
	if p.path != "" {
		return base + " (pt" + p.chunk + ") (" + p.path + ")"
	}
	if p.chunk == "1" {
		return base
	}
	return base + " (pt" + p.chunk + ")"
}

func parsePart(title, base string) (partIdentity, bool, bool) {
	p := partIdentity{chunk: "1"}
	if !strings.HasPrefix(title, base) {
		return p, false, false
	}
	rest := strings.TrimPrefix(title, base)
	old := strings.HasSuffix(rest, " [old]")
	if old {
		rest = strings.TrimSuffix(rest, " [old]")
	}
	numbered := false
	if strings.HasPrefix(rest, " (pt") {
		end := strings.IndexByte(rest, ')')
		if end < 0 {
			return p, old, false
		}
		digits := rest[4:end]
		if !isDigits(digits) || digits[0] == '0' {
			return p, old, false
		}
		p.chunk = digits
		numbered = true
		rest = rest[end+1:]
	}
	if rest == "" {
		return p, old, true
	}
	if numbered && strings.HasPrefix(rest, " (") && strings.HasSuffix(rest, ")") && isLetters(rest[2:len(rest)-1]) {
		p.path = rest[2 : len(rest)-1]
		return p, old, true
	}
	for rest != "" {
		switch {
		case strings.HasPrefix(rest, " (split1)"):
			p.path += "a"
			rest = strings.TrimPrefix(rest, " (split1)")
		case strings.HasPrefix(rest, " (split2)"):
			p.path += "b"
			rest = strings.TrimPrefix(rest, " (split2)")
		default:
			return p, old, false
		}
	}
	return p, old, true
}

func partChild(base, name string, i int) string {
	p, _, _ := parsePart(name, base)
	p.path += string(rune('a' + i - 1))
	return p.title(base)
}

func partDescendant(base, title, parent string) bool {
	child, old, ok := parsePart(title, base)
	ancestor, _, valid := parsePart(parent, base)
	return ok && valid && !old && child.chunk == ancestor.chunk && len(child.path) > len(ancestor.path) && strings.HasPrefix(child.path, ancestor.path)
}

func isLetters(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < 'a' || c > 'b' {
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
