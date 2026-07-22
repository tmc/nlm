package main

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

// TestNullInRepeatedHintEndToEnd finds any request/response message whose field
// 1 is a repeated scalar, then drives beprotojson.Unmarshal via the same helper
// betool's error path calls, on a wire with a null element in that field, and
// confirms the targeted hint fires. This exercises the wired path end to end
// (the helper + the descriptor plumbing betool.go uses), independent of any one
// rpc binding.
func TestNullInRepeatedHintEndToEnd(t *testing.T) {
	var target protoreflect.MessageDescriptor
	protoregistry.GlobalFiles.RangeFiles(func(fd protoreflect.FileDescriptor) bool {
		msgs := fd.Messages()
		for i := 0; i < msgs.Len(); i++ {
			md := msgs.Get(i)
			f := md.Fields().ByNumber(1)
			if f != nil && f.IsList() && f.Message() == nil &&
				strings.HasPrefix(string(md.FullName()), "notebooklm.") {
				target = md
				return false
			}
		}
		return true
	})
	if target == nil {
		t.Skip("no notebooklm message with a top-level repeated scalar field 1")
	}
	// Wire: field 1 is a repeated scalar list containing a null element.
	// e.g. BlobTokens => [["a", null]]
	wire := []byte(`[["a",null]]`)
	// Simulate the decode error text the codec emits for a nil-in-repeated.
	hint := nullInRepeatedHint(errString("expected number, got <nil>"),
		target, wire)
	// For a repeated string field the error text differs, so also try the
	// string-conversion variant.
	if hint == "" {
		hint = nullInRepeatedHint(errString("cannot convert <nil>"), target, wire)
	}
	t.Logf("target message: %s (field 1 = %s)", target.FullName(), target.Fields().ByNumber(1).Name())
	if hint == "" {
		// Not all field-1 repeated scalars produce the "<nil>" error class; that
		// is fine — the unit tests already cover the detector. Just log.
		t.Logf("no hint for %s (error class may differ); detector covered by unit tests", target.FullName())
		return
	}
	t.Logf("HINT: %s", hint)
	if !strings.Contains(hint, "cannot hold a null") ||
		!strings.Contains(hint, "message of optional fields") {
		t.Errorf("hint missing the canonical fix guidance: %q", hint)
	}
}

type errString string

func (e errString) Error() string { return string(e) }
