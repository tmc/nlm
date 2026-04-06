package batchexecute

import (
	"encoding/json"
	"strings"
)

// unescapeResponseData converts a batchexecute inner payload string to valid
// JSON RawMessage. The server returns position-2 data as a JSON-encoded string
// that may contain additional escape layers. For example:
//
//	Wire: [["wrb.fr","rpcId","[[[\\\"uuid\\\"]]]",...]]
//
// After outer json.Unmarshal, the Go string is: [[[\\"uuid\\"]]]
// which still contains literal backslash-quote sequences.
//
// This function iteratively unescapes until valid JSON is produced.
func unescapeResponseData(s string) json.RawMessage {
	// Fast path: if the string is already valid JSON, use it directly.
	if json.Valid([]byte(s)) {
		return json.RawMessage(s)
	}

	// The string may still contain escape sequences from nested encoding.
	// Try iterative unescaping (up to 3 rounds).
	current := s
	for range 3 {
		// Try json.Unmarshal as a JSON string to unescape one layer.
		var unescaped string
		if err := json.Unmarshal([]byte(`"`+current+`"`), &unescaped); err == nil {
			if json.Valid([]byte(unescaped)) {
				return json.RawMessage(unescaped)
			}
			current = unescaped
			continue
		}

		// Manual unescape for cases where embedded quotes prevent
		// wrapping in quotes for json.Unmarshal.
		replaced := manualUnescape(current)
		if replaced == current {
			break
		}
		if json.Valid([]byte(replaced)) {
			return json.RawMessage(replaced)
		}
		current = replaced
	}

	// Final check after all unescaping rounds.
	if json.Valid([]byte(current)) {
		return json.RawMessage(current)
	}

	// Last resort: return the original string as a JSON string value
	// so downstream consumers get something parseable.
	b, _ := json.Marshal(s)
	return json.RawMessage(b)
}

// manualUnescape replaces literal JSON escape sequences that appear in
// partially-escaped batchexecute responses.
func manualUnescape(s string) string {
	if !strings.ContainsRune(s, '\\') {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			switch s[i+1] {
			case '"':
				b.WriteByte('"')
				i++
			case '\\':
				b.WriteByte('\\')
				i++
			case 'n':
				b.WriteByte('\n')
				i++
			case 't':
				b.WriteByte('\t')
				i++
			case 'r':
				b.WriteByte('\r')
				i++
			case 'u':
				// Pass through unicode escapes as-is (they're valid JSON)
				b.WriteByte(s[i])
			default:
				b.WriteByte(s[i])
			}
		} else {
			b.WriteByte(s[i])
		}
	}
	return b.String()
}
