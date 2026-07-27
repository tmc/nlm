package betool

import (
	"errors"
	"strings"
	"testing"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

func TestNullInRepeatedHint(t *testing.T) {
	md := (&pb.Project{}).ProtoReflect().Descriptor()

	// Project.flags is field 10, repeated bool. A null element there is the
	// canonical "repeated cannot hold null" case.
	// Positions [0..9] then [10] = flags with a null element.
	wire := []byte(`["t",null,"id","x",null,null,null,null,null,[true,null,true]]`)
	err := errors.New("beprotojson: field flags: expected number, got <nil>")

	hint := nullInRepeatedHint(err, md, wire)
	if hint == "" {
		t.Fatalf("expected a hint for null-in-repeated, got none")
	}
	if !strings.Contains(hint, "field 10") || !strings.Contains(hint, "flags") {
		t.Errorf("hint should name field 10 (flags): %q", hint)
	}
	if !strings.Contains(hint, "message of optional fields") {
		t.Errorf("hint should suggest the fix: %q", hint)
	}
}

func TestNullInRepeatedHintNoFalsePositive(t *testing.T) {
	md := (&pb.Project{}).ProtoReflect().Descriptor()

	// A wire with no null-in-repeated-scalar and an unrelated error must not
	// produce a hint.
	wire := []byte(`["t",null,"id","x",null,null,null,null,null,[true,true,true]]`)
	if got := nullInRepeatedHint(errors.New("some other error"), md, wire); got != "" {
		t.Errorf("unrelated error should yield no hint, got %q", got)
	}
	// Right error class but no actual null in a repeated scalar → no hint.
	err := errors.New("beprotojson: field x: expected number, got <nil>")
	if got := nullInRepeatedHint(err, md, wire); got != "" {
		t.Errorf("no null-in-repeated present, expected no hint, got %q", got)
	}
}
