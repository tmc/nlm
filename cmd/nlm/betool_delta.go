package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/tmc/nlm/internal/beprotojson"
)

// fieldDelta describes one position in the original wire payload that the proto
// view fails to preserve, so a round-trip through the proto type would drop or
// reshape it. It is the actionable output of --verify: it points at exactly
// which wire data the current descriptors do not model.
//
// Findings are located structurally (path + the lost value) and, when the
// walk has a descriptor, by the static message field that owns the position.
type fieldDelta struct {
	// Path is the positional path into the wire array, e.g. "[0][0][2][3]".
	Path string `json:"path"`
	// Kind is "unmodeled" (the value is dropped entirely by the proto view) or
	// "shape" (it survives but with a different shape — a nested array
	// flattened to a scalar or vice versa).
	Kind string `json:"kind"`
	// Name identifies the static descriptor field at the lost position. A
	// repeated message element uses a parent-field description because the loss
	// is the whole element, not a field within it.
	Name string `json:"name,omitempty"`
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
	// beprotojson marshals positional messages to their declared width, while
	// captures commonly elide trailing null slots. Normalize only trailing
	// nulls, recursively; interior nulls carry positional meaning and must
	// remain visible to the comparator.
	ov = trimTrailingNulls(ov)
	rv = trimTrailingNulls(rv)
	var out []fieldDelta
	desc := msg.ProtoReflect().Descriptor()
	if topWasUnwrapped(ov) {
		if field := desc.Fields().ByNumber(1); field != nil && field.IsList() {
			// The original wire dropped every trailing null field, collapsing
			// down to just position 0 (the repeated field's element list), so
			// the first path component is still the response field position.
			// The remarshal does NOT drop trailing nulls (Marshal always pads
			// to the highest declared field number), so unwrapping it the same
			// single-element way it unwraps the original would only fire when
			// the response has exactly one field total; extract field 1's
			// value directly instead so both sides compare at the same depth
			// regardless of how many trailing fields the response declares.
			rvField0 := fieldOneValue(rv)
			if field.Message() != nil {
				diffRepeatedMessage(unwrapTop(ov), rvField0, "[0]", &out, field)
			} else {
				diffRaw(unwrapTop(ov), rvField0, "[0]", &out, descriptorFieldName(desc, 1))
			}
			return out, nil
		}
	}
	if fieldOneWrapsRepeatedMessage(desc) {
		diffValue(ov, rv, "[0]", &out, desc)
	} else {
		diffValue(unwrapTop(ov), unwrapTop(rv), "[0]", &out, desc)
	}
	return out, nil
}

// fieldOneWrapsRepeatedMessage reports whether field 1 is a singular message
// whose own field 1 is a repeated message. For that shape, the outer
// single-element array is the response's positional wrapper, not an extra
// batchexecute envelope.
func fieldOneWrapsRepeatedMessage(desc protoreflect.MessageDescriptor) bool {
	if desc == nil {
		return false
	}
	outer := desc.Fields().ByNumber(1)
	if outer == nil || outer.IsList() || outer.Message() == nil {
		return false
	}
	inner := outer.Message().Fields().ByNumber(1)
	return inner != nil && inner.IsList() && inner.Message() != nil
}

func trimTrailingNulls(v any) any {
	switch x := v.(type) {
	case []any:
		out := make([]any, len(x))
		for i, item := range x {
			out[i] = trimTrailingNulls(item)
		}
		for len(out) > 0 && out[len(out)-1] == nil {
			out = out[:len(out)-1]
		}
		return out
	case map[string]any:
		for key, item := range x {
			x[key] = trimTrailingNulls(item)
		}
	}
	return v
}

func topWasUnwrapped(v any) bool {
	arr, ok := v.([]any)
	if !ok || len(arr) != 1 {
		return false
	}
	_, ok = arr[0].([]any)
	return ok
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

// fieldOneValue returns position 0 (field 1) of a positional response array,
// or nil if v is not shaped like one. Unlike unwrapTop, this does not require
// the array to have exactly one element — it is used on the remarshal side,
// which (unlike the original wire) always carries every declared trailing
// field as an explicit null rather than dropping them.
func fieldOneValue(v any) any {
	if arr, ok := v.([]any); ok && len(arr) > 0 {
		return arr[0]
	}
	return nil
}

// diffValue compares an original wire value a against the proto-re-encoded
// value b at the same position, recording the positions the proto view lost.
func diffValue(a, b any, path string, out *[]fieldDelta, desc protoreflect.MessageDescriptor) {
	if isNull(a) {
		return // nothing carried on the wire here; b being non-null is only padding
	}

	if desc == nil {
		diffRaw(a, b, path, out, "")
		return
	}
	if isNull(b) {
		addDelta(out, path, "unmodeled", a, string(desc.Name()))
		return
	}
	if oneof := deltaShapeUnion(desc); oneof != nil {
		if field := deltaShapeCase(oneof, a); field != nil {
			diffShapeUnionValue(a, b, path, out, field)
		}
		return
	}
	if _, aObject := a.(map[string]any); aObject {
		if _, bObject := b.(map[string]any); bObject {
			diffObjectMessage(a, b, path, out, desc)
			return
		}
	}
	aArr, aIsArr := a.([]any)
	bArr, bIsArr := b.([]any)
	if !aIsArr || !bIsArr {
		addDelta(out, path, "shape", a, string(desc.Name()))
		return
	}
	for i := range max(len(aArr), len(bArr)) {
		var av, bv any
		if i < len(aArr) {
			av = aArr[i]
		}
		if i < len(bArr) {
			bv = bArr[i]
		}
		field := desc.Fields().ByNumber(protoreflect.FieldNumber(i + 1))
		fieldName := descriptorFieldName(desc, i+1)
		switch {
		case field == nil:
			diffRaw(av, bv, fmt.Sprintf("%s[%d]", path, i), out, fieldName)
		case field.IsList() && field.Message() != nil:
			diffRepeatedMessage(av, bv, fmt.Sprintf("%s[%d]", path, i), out, field)
		case field.Message() != nil && !isFlattenedWellKnown(field.Message()):
			diffMessage(av, bv, fmt.Sprintf("%s[%d]", path, i), out, field)
		default:
			// Scalars, repeated scalars, and flattened well-known types are
			// leaves for descriptor naming. In particular, do not interpret
			// Timestamp's [seconds,nanos] tuple as message fields.
			diffRaw(av, bv, fmt.Sprintf("%s[%d]", path, i), out, fieldName)
		}
	}
}

// diffObjectMessage compares an object-encoded message, whose wire keys are
// field numbers, without treating the object as a positional array.
func diffObjectMessage(a, b any, path string, out *[]fieldDelta, desc protoreflect.MessageDescriptor) {
	am := a.(map[string]any)
	bm := b.(map[string]any)
	keys := make([]string, 0, len(am)+len(bm))
	seen := make(map[string]bool, len(am)+len(bm))
	for key := range am {
		seen[key] = true
		keys = append(keys, key)
	}
	for key := range bm {
		if !seen[key] {
			keys = append(keys, key)
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		a, _ := strconv.Atoi(keys[i])
		b, _ := strconv.Atoi(keys[j])
		return a < b
	})
	for _, key := range keys {
		number, err := strconv.Atoi(key)
		if err != nil {
			continue
		}
		field := desc.Fields().ByNumber(protoreflect.FieldNumber(number))
		name := descriptorFieldName(desc, number)
		av, aOK := am[key]
		bv, bOK := bm[key]
		if !aOK {
			av = nil
		}
		if !bOK {
			bv = nil
		}
		switch {
		case field == nil:
			diffRaw(av, bv, fmt.Sprintf("%s[%s]", path, key), out, name)
		case field.IsList() && field.Message() != nil:
			diffRepeatedMessage(av, bv, fmt.Sprintf("%s[%s]", path, key), out, field)
		case field.Message() != nil && !isFlattenedWellKnown(field.Message()):
			diffMessage(av, bv, fmt.Sprintf("%s[%s]", path, key), out, field)
		default:
			diffRaw(av, bv, fmt.Sprintf("%s[%s]", path, key), out, name)
		}
	}
}

// deltaShapeUnion mirrors beprotojson's positional shape-union predicate. A
// shape union is encoded as the selected case's bare value, so its value must
// be compared at the union's path rather than interpreted as a field-numbered
// positional message.
func deltaShapeUnion(md protoreflect.MessageDescriptor) protoreflect.OneofDescriptor {
	if md == nil || md.Oneofs().Len() != 1 {
		return nil
	}
	oneof := md.Oneofs().Get(0)
	if oneof.IsSynthetic() || oneof.Fields().Len() != md.Fields().Len() {
		return nil
	}
	return oneof
}

func deltaShapeCase(oneof protoreflect.OneofDescriptor, value any) protoreflect.FieldDescriptor {
	for i := 0; i < oneof.Fields().Len(); i++ {
		field := oneof.Fields().Get(i)
		if deltaShapeMatches(field, value) {
			return field
		}
	}
	return nil
}

func deltaShapeMatches(field protoreflect.FieldDescriptor, value any) bool {
	switch v := value.(type) {
	case []any:
		if field.IsList() {
			return true
		}
		if field.Message() != nil {
			fields := field.Message().Fields()
			if fields.Len() == 0 || len(v) == 0 {
				return true
			}
			return deltaShapeMatches(fields.Get(0), v[0])
		}
		if len(v) == 1 {
			return deltaShapeMatches(field, v[0])
		}
		return false
	case string:
		return field.Kind() == protoreflect.StringKind || field.Kind() == protoreflect.BytesKind
	case bool:
		return field.Kind() == protoreflect.BoolKind
	case float64:
		switch field.Kind() {
		case protoreflect.Int32Kind, protoreflect.Int64Kind, protoreflect.Sint32Kind,
			protoreflect.Sint64Kind, protoreflect.Sfixed32Kind, protoreflect.Sfixed64Kind,
			protoreflect.Uint32Kind, protoreflect.Uint64Kind, protoreflect.Fixed32Kind,
			protoreflect.Fixed64Kind, protoreflect.FloatKind, protoreflect.DoubleKind,
			protoreflect.EnumKind:
			return true
		}
	}
	return false
}

func diffShapeUnionValue(a, b any, path string, out *[]fieldDelta, field protoreflect.FieldDescriptor) {
	switch {
	case field.IsList() && field.Message() != nil:
		diffRepeatedMessage(a, b, path, out, field)
	case field.Message() != nil && !isFlattenedWellKnown(field.Message()):
		diffMessage(a, b, path, out, field)
	default:
		diffRaw(a, b, path, out, descriptorFieldName(field.ContainingMessage(), int(field.Number())))
	}
}

func diffRaw(a, b any, path string, out *[]fieldDelta, name string) {
	if isNull(a) {
		return
	}
	if isNull(b) {
		addDelta(out, path, "unmodeled", a, name)
		return
	}
	aArr, aIsArr := a.([]any)
	bArr, bIsArr := b.([]any)
	if aIsArr != bIsArr {
		addDelta(out, path, "shape", a, name)
		return
	}
	if aIsArr {
		for i := range max(len(aArr), len(bArr)) {
			var av, bv any
			if i < len(aArr) {
				av = aArr[i]
			}
			if i < len(bArr) {
				bv = bArr[i]
			}
			diffRaw(av, bv, fmt.Sprintf("%s[%d]", path, i), out, name)
		}
		return
	}
	if !jsonEqualValues(a, b) {
		addDelta(out, path, "value", a, name)
	}
}

func diffMessage(a, b any, path string, out *[]fieldDelta, field protoreflect.FieldDescriptor) {
	if isNull(a) {
		return
	}
	name := descriptorFieldName(field.ContainingMessage(), int(field.Number()))
	if isNull(b) {
		addDelta(out, path, "unmodeled", a, name)
		return
	}
	if _, ok := a.(map[string]any); ok {
		if _, ok := b.(map[string]any); ok {
			diffValue(a, b, path, out, field.Message())
			return
		}
	}
	if _, aIsArr := a.([]any); !aIsArr {
		addDelta(out, path, "shape", a, name)
		return
	}
	if _, bIsArr := b.([]any); !bIsArr {
		addDelta(out, path, "shape", a, name)
		return
	}
	diffValue(a, b, path, out, field.Message())
}

func diffRepeatedMessage(a, b any, path string, out *[]fieldDelta, field protoreflect.FieldDescriptor) {
	if isNull(a) {
		return
	}
	elementName := fmt.Sprintf("%s.%s[*]: element does not fit %s",
		field.ContainingMessage().Name(), field.Name(), field.Message().Name())
	aArr, aIsArr := a.([]any)
	bArr, bIsArr := b.([]any)
	if !aIsArr || !bIsArr {
		addDelta(out, path, "shape", a, descriptorFieldName(field.ContainingMessage(), int(field.Number())))
		return
	}
	if oneof := deltaShapeUnion(field.Message()); oneof != nil && len(bArr) == 1 {
		if selected := deltaShapeCase(oneof, aArr); selected != nil {
			diffShapeUnionValue(aArr, bArr[0], path, out, selected)
			return
		}
	}
	for i := range max(len(aArr), len(bArr)) {
		var av, bv any
		if i < len(aArr) {
			av = aArr[i]
		}
		if i < len(bArr) {
			bv = bArr[i]
		}
		elementPath := fmt.Sprintf("%s[%d]", path, i)
		if isNull(av) {
			continue
		}
		if isNull(bv) {
			addDelta(out, elementPath, "unmodeled", av, elementName)
			continue
		}
		if oneof := deltaShapeUnion(field.Message()); oneof != nil {
			if selected := deltaShapeCase(oneof, av); selected != nil {
				diffShapeUnionValue(av, bv, elementPath, out, selected)
			}
			continue
		}
		_, avOK := av.([]any)
		_, bvOK := bv.([]any)
		if !avOK || !bvOK {
			addDelta(out, elementPath, "shape", av, elementName)
			continue
		}
		diffValue(av, bv, elementPath, out, field.Message())
	}
}

func addDelta(out *[]fieldDelta, path, kind string, original any, name string) {
	*out = append(*out, fieldDelta{Path: pathOrRoot(path), Kind: kind, Name: name, Original: mustJSON(original)})
}

func descriptorFieldName(desc protoreflect.MessageDescriptor, number int) string {
	if desc == nil {
		return ""
	}
	if field := desc.Fields().ByNumber(protoreflect.FieldNumber(number)); field != nil {
		return fmt.Sprintf("%s.%s#%d", desc.Name(), field.Name(), field.Number())
	}
	return fmt.Sprintf("%s.unknown_%d", desc.Name(), number)
}

func isFlattenedWellKnown(desc protoreflect.MessageDescriptor) bool {
	if desc == nil {
		return false
	}
	switch desc.FullName() {
	case "google.protobuf.Timestamp", "google.protobuf.StringValue", "google.protobuf.Int32Value":
		return true
	default:
		return false
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
	// Name identifies the descriptor field or message type responsible for the
	// group, for example SourceMetadata.unknown_4 or a repeated-element fit.
	Name string `json:"name"`
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
		key := fmt.Sprintf("%d\x00%s\x00%s", len(idx), d.Kind, d.Name)
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
			Name:    a.example.Name,
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
