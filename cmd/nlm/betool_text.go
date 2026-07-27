package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/tmc/nlm/internal/batchexecute"
)

// The default betool output is this human-readable text; --json restores the
// full structured JSON. The text form favors quick reading in a terminal: one
// labeled block per RPC, compact args/data, and a one-line verify verdict.

// writeRequestText summarizes a decoded wire request: the auth token (if any)
// and one line per RPC with its id and compacted args.
func writeRequestText(req *batchexecute.WireRequest) error {
	var b strings.Builder
	fmt.Fprintf(&b, "request  %d rpc(s)", len(req.RPCs))
	if req.At != "" {
		fmt.Fprintf(&b, "  at=%s", req.At)
	}
	b.WriteByte('\n')
	for i, r := range req.RPCs {
		fmt.Fprintf(&b, "  [%d] %-8s args: %s", i, r.ID, compactOrRaw(r.Args))
		if r.Index != "" {
			fmt.Fprintf(&b, "  index=%s", r.Index)
		}
		b.WriteByte('\n')
	}
	return writeText(b.String())
}

// writeResponseText summarizes a decoded wire response: one line per RPC with
// its id, index, and a preview of the decoded data (or its error).
func writeResponseText(resp *batchexecute.WireResponse) error {
	var b strings.Builder
	fmt.Fprintf(&b, "response  %d rpc(s)\n", len(resp.Responses))
	for _, r := range resp.Responses {
		fmt.Fprintf(&b, "  [%d] %-8s", r.Index, r.ID)
		switch {
		case r.Error != "":
			fmt.Fprintf(&b, "  error: %s", r.Error)
		case len(r.Data) > 0:
			fmt.Fprintf(&b, "  data: %s", preview(compactOrRaw(r.Data), 120))
		default:
			b.WriteString("  (no data)")
		}
		b.WriteByte('\n')
	}
	return writeText(b.String())
}

// writeProtoText renders decoded proto envelopes: the rpc_id and method, the
// message as indented proto JSON, and a one-line verify verdict when present.
func writeProtoText(envs []protoEnvelope) error {
	var b strings.Builder
	for i, e := range envs {
		if i > 0 {
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "%s  %s\n", e.RPCID, e.Method)
		fmt.Fprintf(&b, "  type: %s\n", e.Type)
		if e.Lossless == nil {
			b.WriteString(indentBlock(prettyJSON(e.Message), "  "))
			b.WriteByte('\n')
		}
		if e.Lossless != nil {
			b.WriteString(verifyLine(e))
			b.WriteByte('\n')
		}
		if e.Inferred != "" {
			b.WriteString(indentBlock(e.Inferred, "  "))
			b.WriteByte('\n')
		}
	}
	return writeText(b.String())
}

// verifyLine renders the round-trip verdict for one envelope: lossless, or a
// LOSSY summary followed by one line per finding group (normalized path, count,
// and distinct-shape count when >1).
func verifyLine(e protoEnvelope) string {
	if e.Lossless != nil && *e.Lossless {
		return "  verify: lossless"
	}
	total := e.MissingCount
	if total == 0 {
		return "  verify: LOSSY (unmodeled wire data)"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "  verify: LOSSY — %s in %s",
		plural(total, "position"), plural(len(e.MissingGroups), "group"))
	for _, g := range e.MissingGroups {
		fmt.Fprintf(&b, "\n    %s  %s  %s  ×%d", g.Path, g.Name, g.Kind, g.Count)
		if g.Shapes > 1 {
			fmt.Fprintf(&b, "  (%d shapes)", g.Shapes)
		}
		if len(g.Example.Original) > 0 {
			fmt.Fprintf(&b, "\n      data: %s", preview(compactOrRaw(g.Example.Original), 96))
		}
	}
	return b.String()
}

// inferMissingGroups renders only the top-level grouped findings. Inference
// is an inspection aid for a terminal, so it must not expand every nested
// message in a large response into a second schema dump.
func inferMissingGroups(groups []deltaGroup) string {
	if len(groups) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("infer-missing:")
	for _, g := range groups {
		typ := inferredValueType(g.Example.Original)
		if strings.Contains(g.Name, "element does not fit") {
			typ = "message"
		}
		fmt.Fprintf(&b, "\n  %s: %s  (%s, %s)", g.Name, typ, g.Kind, plural(g.Count, "position"))
		if len(g.Example.Original) > 0 {
			fmt.Fprintf(&b, "\n    example: %s", preview(compactOrRaw(g.Example.Original), 96))
		}
	}
	return b.String()
}

func inferredValueType(raw json.RawMessage) string {
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return "unknown"
	}
	return inferredAnyType(v)
}

func inferredAnyType(v any) string {
	switch x := v.(type) {
	case nil:
		return "unknown"
	case string:
		return "string"
	case bool:
		return "bool"
	case float64:
		if x == float64(int64(x)) {
			return "int64"
		}
		return "double"
	case []any:
		if len(x) == 0 {
			return "repeated unknown"
		}
		first := inferredAnyType(x[0])
		for _, item := range x[1:] {
			if inferredAnyType(item) != first {
				return "message"
			}
		}
		return "repeated " + first
	case map[string]any:
		return "message"
	default:
		return "unknown"
	}
}

// plural formats n with noun, adding "s" unless n == 1.
func plural(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// writeText writes s to stdout, ensuring a single trailing newline.
func writeText(s string) error {
	if !strings.HasSuffix(s, "\n") {
		s += "\n"
	}
	_, err := os.Stdout.WriteString(s)
	return err
}

// compactOrRaw compacts JSON to a single line; if the bytes are not valid JSON
// it returns them unchanged so nothing is hidden.
func compactOrRaw(raw json.RawMessage) string {
	if len(raw) == 0 {
		return "null"
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		return string(raw)
	}
	return buf.String()
}

// prettyJSON indents JSON for reading; invalid JSON is returned unchanged.
func prettyJSON(raw json.RawMessage) string {
	var buf bytes.Buffer
	if err := json.Indent(&buf, raw, "", "  "); err != nil {
		return string(raw)
	}
	return buf.String()
}

// indentBlock prefixes every line of s with prefix.
func indentBlock(s, prefix string) string {
	lines := strings.Split(s, "\n")
	for i, ln := range lines {
		if ln != "" {
			lines[i] = prefix + ln
		}
	}
	return strings.Join(lines, "\n")
}

// preview truncates s to at most n runes, appending an ellipsis and the full
// length when it had to cut.
func preview(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return fmt.Sprintf("%s… (%d bytes)", string(r[:n]), len(s))
}
