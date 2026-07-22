package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"google.golang.org/protobuf/reflect/protoreflect"
)

// nullInRepeatedHint inspects a beprotojson.Unmarshal failure and, when the
// failure is a null element landing in a repeated scalar field, returns a
// targeted, human-actionable explanation. A repeated scalar (repeated int64,
// repeated string, …) cannot hold a null element, so a single null slot in an
// otherwise-numeric tuple aborts the whole decode — which otherwise surfaces
// only as a generic "expected number, got <nil>" far from its cause, or as a
// whole-element "does not fit" cascade in --verify.
//
// The hint names the offending field and the fix: model the tuple as a message
// of optional fields (a null slot then round-trips as an absent field). It
// returns "" when the error is not of this class, so callers can append it
// only when present.
//
// This is a pure diagnostic helper: it reads the descriptor and the raw wire
// and never mutates anything. It is intentionally self-contained (no edits to
// the decoder or the delta comparator) so it can be wired in from a single
// call site in the error path — betool.go appends it at the request and
// response Unmarshal error sites, e.g.:
//
//	if err := beprotojson.Unmarshal(r.Data, msg); err != nil {
//		if hint := nullInRepeatedHint(err, method.Response.Descriptor(), r.Data); hint != "" {
//			return fmt.Errorf("… %w (%s)", err, hint)
//		}
//		return …
//	}
//
// The corresponding verify finding kind is "null-in-repeated" (distinct from
// the comparator's padded-vs-elided normalization, which produces no finding).
func nullInRepeatedHint(err error, md protoreflect.MessageDescriptor, wire []byte) string {
	if err == nil || md == nil {
		return ""
	}
	// The decoder reports this class as "expected number, got <nil>" (or the
	// %T rendering of a nil interface). Only proceed for that shape.
	msg := err.Error()
	if !strings.Contains(msg, "got <nil>") &&
		!strings.Contains(msg, "got string \"<nil>\"") {
		return ""
	}

	// Parse the wire and look for a repeated scalar field whose positional slot
	// carries a null. We scan the top-level message positionally.
	var arr []any
	if json.Unmarshal(wire, &arr) != nil {
		return ""
	}
	if f := findNullInRepeatedScalar(md, arr); f != nil {
		return fmt.Sprintf(
			"null element in repeated field %d (%s.%s): a repeated %s cannot hold a null; "+
				"model this tuple as a message of optional fields so the null slot round-trips as null",
			f.Number(), md.Name(), f.Name(), f.Kind())
	}
	return ""
}

// findNullInRepeatedScalar returns the first repeated-scalar field descriptor
// whose corresponding wire slot is a list containing a null element. It walks
// one level; nested messages are out of scope for this cheap top-level hint.
func findNullInRepeatedScalar(md protoreflect.MessageDescriptor, arr []any) protoreflect.FieldDescriptor {
	fields := md.Fields()
	for i, v := range arr {
		list, ok := v.([]any)
		if !ok {
			continue
		}
		fd := fields.ByNumber(protoreflect.FieldNumber(i + 1))
		if fd == nil || !fd.IsList() || fd.Message() != nil {
			continue // not a repeated scalar
		}
		for _, elem := range list {
			if elem == nil {
				return fd
			}
		}
	}
	return nil
}
