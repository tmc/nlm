package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"google.golang.org/protobuf/proto"

	"github.com/tmc/nlm/internal/beprotojson"
)

// fieldDelta describes one position in the original wire payload that the proto
// view fails to preserve, so a round-trip through the proto type would drop or
// reshape it. It is the actionable output of --verify: it points at exactly
// which wire data the current descriptors do not model.
//
// Findings are located structurally (path + the lost value), which is always
// accurate. Naming each finding with its proto message + field number is left
// to a future dynamic-descriptor pass; see docs/betool-proto-reifier.md.
type fieldDelta struct {
	// Path is the positional path into the wire array, e.g. "[0][0][2][3]".
	Path string `json:"path"`
	// Kind is "unmodeled" (the value is dropped entirely by the proto view) or
	// "shape" (it survives but with a different shape — a nested array
	// flattened to a scalar or vice versa).
	Kind string `json:"kind"`
	// Original is the wire value at this position that the proto view loses.
	Original json.RawMessage `json:"original"`
}

// diffWireAgainstProto reports the wire positions that the proto view of msg
// fails to preserve. Ground truth for "what the descriptor models" is
// beprotojson's own re-marshal of the decoded message: comparing the original
// wire against that output finds every dropped or reshaped value with no
// framing guesswork.
func diffWireAgainstProto(original []byte, msg proto.Message) ([]fieldDelta, error) {
	remarshaled, err := beprotojson.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("re-encode proto to wire: %w", err)
	}
	var ov, rv any
	if err := json.Unmarshal(original, &ov); err != nil {
		return nil, fmt.Errorf("parse original wire: %w", err)
	}
	if err := json.Unmarshal(remarshaled, &rv); err != nil {
		return nil, fmt.Errorf("parse re-encoded wire: %w", err)
	}
	var out []fieldDelta
	diffValue(unwrapTop(ov), unwrapTop(rv), "[0]", &out)
	return out, nil
}

// unwrapTop removes the single-element positional wrapper batchexecute puts
// around a message whose sole populated top-level field is repeated, so both
// sides are compared at the same depth.
func unwrapTop(v any) any {
	if arr, ok := v.([]any); ok && len(arr) == 1 {
		if _, inner := arr[0].([]any); inner {
			return arr[0]
		}
	}
	return v
}

// diffValue compares an original wire value a against the proto-re-encoded
// value b at the same position, recording the positions the proto view lost.
func diffValue(a, b any, path string, out *[]fieldDelta) {
	if isNull(a) {
		return // nothing carried on the wire here; b being non-null is only padding
	}

	aArr, aIsArr := a.([]any)
	bArr, bIsArr := b.([]any)

	// Original has a value the proto dropped (null or absent).
	if isNull(b) {
		*out = append(*out, fieldDelta{Path: pathOrRoot(path), Kind: "unmodeled", Original: mustJSON(a)})
		return
	}

	// Shape divergence: one side is an array and the other is not.
	if aIsArr != bIsArr {
		*out = append(*out, fieldDelta{Path: pathOrRoot(path), Kind: "shape", Original: mustJSON(a)})
		return
	}

	// Both arrays: recurse position-by-position.
	if aIsArr && bIsArr {
		n := max(len(aArr), len(bArr))
		for i := range n {
			var av, bv any
			if i < len(aArr) {
				av = aArr[i]
			}
			if i < len(bArr) {
				bv = bArr[i]
			}
			diffValue(av, bv, fmt.Sprintf("%s[%d]", path, i), out)
		}
		return
	}

	// Both scalars: report a value change (rare; usually enum/format handling).
	if !jsonEqualValues(a, b) {
		*out = append(*out, fieldDelta{Path: pathOrRoot(path), Kind: "value", Original: mustJSON(a)})
	}
}

func isNull(v any) bool { return v == nil }

func pathOrRoot(p string) string {
	if p == "" {
		return "$"
	}
	return p
}

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage(`null`)
	}
	return json.RawMessage(b)
}

func jsonEqualValues(a, b any) bool {
	ab, _ := json.Marshal(a)
	bb, _ := json.Marshal(b)
	return string(ab) == string(bb)
}

// deltaGroup collapses findings that occupy the same structural position but
// differ only in array indices — e.g. one dropped field repeated once per
// element of a repeated list. It is the default --verify view: it keeps the
// dedup signal (N occurrences of one gap) without hiding genuinely distinct
// gaps, which surface as separate groups or as multiple shapes within a group.
type deltaGroup struct {
	// Path is the normalized structural path with array indices replaced by
	// "*", e.g. "[0][*]" or "[0][0][1][*][2][3]".
	Path string `json:"path"`
	// Kind is the finding kind shared by the group ("unmodeled"/"shape"/"value").
	Kind string `json:"kind"`
	// Count is how many concrete positions collapsed into this group.
	Count int `json:"count"`
	// Shapes is the number of structurally distinct dropped values in the group
	// (>1 means the same position drops different shapes, e.g. user vs assistant
	// turns), a hint that more than one schema gap hides here.
	Shapes int `json:"shapes"`
	// Example is one concrete finding from the group, verbatim.
	Example fieldDelta `json:"example"`
}

// groupDeltas collapses deltas that share a structural position — same path
// length and kind — counting occurrences and distinct value-shapes. The
// group's displayed path stars only the index components that actually vary
// across its members, so constant landmarks are preserved: four findings at
// [0][0][1][0..3][2][3] display as [0][0][1][*][2][3], not [*][*][*][*][*][*].
// Groups are returned in first-seen order so output is stable.
func groupDeltas(deltas []fieldDelta) []deltaGroup {
	type acc struct {
		example fieldDelta
		kind    string
		indexes [][]int // per-member index components, same arity
		shapes  map[string]bool
	}
	order := []string{}
	byKey := map[string]*acc{}
	for _, d := range deltas {
		idx := pathIndexes(d.Path)
		key := fmt.Sprintf("%d\x00%s", len(idx), d.Kind)
		a, ok := byKey[key]
		if !ok {
			a = &acc{example: d, kind: d.Kind, shapes: map[string]bool{}}
			byKey[key] = a
			order = append(order, key)
		}
		a.indexes = append(a.indexes, idx)
		a.shapes[shapeKey(d.Original)] = true
	}
	out := make([]deltaGroup, 0, len(order))
	for _, key := range order {
		a := byKey[key]
		out = append(out, deltaGroup{
			Path:    collapsePath(a.indexes),
			Kind:    a.kind,
			Count:   len(a.indexes),
			Shapes:  len(a.shapes),
			Example: a.example,
		})
	}
	return out
}

// pathIndexes extracts the integer index components from a path like
// "[0][0][1][3][2][3]".
func pathIndexes(path string) []int {
	var out []int
	for part := range strings.SplitSeq(path, "[") {
		part = strings.TrimSuffix(part, "]")
		if part == "" {
			continue
		}
		n := 0
		for _, c := range part {
			n = n*10 + int(c-'0')
		}
		out = append(out, n)
	}
	return out
}

// collapsePath renders a path where each component is its shared literal value
// if constant across all members, or "*" if it varies.
func collapsePath(members [][]int) string {
	if len(members) == 0 {
		return ""
	}
	arity := len(members[0])
	var b strings.Builder
	for i := range arity {
		v := members[0][i]
		varies := false
		for _, m := range members[1:] {
			if m[i] != v {
				varies = true
				break
			}
		}
		if varies {
			b.WriteString("[*]")
		} else {
			fmt.Fprintf(&b, "[%d]", v)
		}
	}
	return b.String()
}

// shapeKey summarizes a dropped value's structure (arrays vs scalars, by
// position) so that structurally identical values share a key while values
// that differ in shape — a populated field 4 vs a populated field 5 — do not.
func shapeKey(raw json.RawMessage) string {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return "?"
	}
	return shapeOf(v)
}

func shapeOf(v any) string {
	switch t := v.(type) {
	case []any:
		parts := make([]string, len(t))
		for i, e := range t {
			parts[i] = shapeOf(e)
		}
		return "[" + strings.Join(parts, ",") + "]"
	case nil:
		return "n"
	case string:
		return "s"
	case bool:
		return "b"
	default: // json numbers
		return "d"
	}
}
