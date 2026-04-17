package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

// readLinesFromReader returns non-empty, whitespace-trimmed lines read from r,
// ignoring blank lines and `#`-prefixed comment lines. Suitable for piping
// `nlm sources <nb> -q` output into commands that accept lists of IDs.
func readLinesFromReader(r io.Reader) ([]string, error) {
	var out []string
	scanner := bufio.NewScanner(r)
	// Allow long single-line IDs (UUIDs are short, but defensive for hex
	// blobs or other future identifier shapes).
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// If a line has whitespace-separated fields (e.g. `nlm sources` default
		// output is a column table), take only the first field — conventionally
		// the ID column.
		if i := strings.IndexAny(line, " \t"); i > 0 {
			line = line[:i]
		}
		out = append(out, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read stdin: %w", err)
	}
	return out, nil
}

// resolveIDList expands a CLI argument into a concrete []string. The argument
// may be:
//   - "-"           read newline-separated IDs from stdin
//   - "a,b,c"       a comma-separated list
//   - ""            the empty slice (caller decides whether that's valid)
//   - anything else a single-element list
//
// When stdin is a TTY and the argument is "-", the function refuses rather
// than blocking on an interactive read — mirrors the confirmAction policy.
func resolveIDList(arg string) ([]string, error) {
	if arg == "-" {
		if isTerminal(os.Stdin) {
			return nil, fmt.Errorf("refusing to read IDs from an interactive stdin; pipe input or pass a list instead")
		}
		return readLinesFromReader(os.Stdin)
	}
	if arg == "" {
		return nil, nil
	}
	if strings.Contains(arg, ",") {
		parts := strings.Split(arg, ",")
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			if s := strings.TrimSpace(p); s != "" {
				out = append(out, s)
			}
		}
		return out, nil
	}
	return []string{arg}, nil
}
