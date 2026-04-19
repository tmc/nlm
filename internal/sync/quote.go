package sync

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// Quoting lets sync bundle files whose contents contain lines that look
// like txtar markers ("-- name --") without corrupting the outer archive.
// The scheme matches github.com/rogpeppe/go-internal/txtar so output round-
// trips through txtar-c -quote and any reader that honors "unquote NAME"
// comment directives.

// needsQuote reports whether data contains an embedded txtar file marker.
func needsQuote(data []byte) bool {
	_, _, after := findFileMarker(data)
	return after != nil
}

// quote prefixes every line with '>' so that no marker line survives.
// The original data can be recovered with unquote. Data must end in '\n'
// and be valid UTF-8.
func quote(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, nil
	}
	if data[len(data)-1] != '\n' {
		return nil, errors.New("data has no final newline")
	}
	if !utf8.Valid(data) {
		return nil, fmt.Errorf("data contains non-UTF-8 characters")
	}
	out := make([]byte, 0, len(data)+len(data)/64+1)
	prev := byte('\n')
	for _, b := range data {
		if prev == '\n' {
			out = append(out, '>')
		}
		out = append(out, b)
		prev = b
	}
	return out, nil
}

// unquote reverses quote. Exported-style helper kept for tests and for
// future read-back paths.
func unquote(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, nil
	}
	if data[0] != '>' || data[len(data)-1] != '\n' {
		return nil, errors.New("data does not appear to be quoted")
	}
	data = bytes.ReplaceAll(data, []byte("\n>"), []byte("\n"))
	data = bytes.TrimPrefix(data, []byte(">"))
	return data, nil
}

var (
	newlineMarker = []byte("\n-- ")
	markerPrefix  = []byte("-- ")
	markerSuffix  = []byte(" --")
)

func findFileMarker(data []byte) (before []byte, name string, after []byte) {
	var i int
	for {
		if name, after = isMarker(data[i:]); name != "" {
			return data[:i], name, after
		}
		j := bytes.Index(data[i:], newlineMarker)
		if j < 0 {
			return data, "", nil
		}
		i += j + 1
	}
}

func isMarker(data []byte) (name string, after []byte) {
	if !bytes.HasPrefix(data, markerPrefix) {
		return "", nil
	}
	if i := bytes.IndexByte(data, '\n'); i >= 0 {
		data, after = data[:i], data[i+1:]
		if i > 0 && data[i-1] == '\r' {
			data = data[:len(data)-1]
		}
	}
	if !bytes.HasSuffix(data, markerSuffix) {
		return "", nil
	}
	return strings.TrimSpace(string(data[len(markerPrefix) : len(data)-len(markerSuffix)])), after
}
