package api

import (
	"reflect"
	"testing"

	method "github.com/tmc/nlm/gen/method"
	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// TestStartDeepResearchEncoderLegacyShape pins the bytes used by the existing
// client. The single real QA9ei web capture uses a context envelope at
// position 0 and null at position 1; changing to that variant requires a live
// server check because starting research is an expensive operation.
func TestStartDeepResearchEncoderLegacyShape(t *testing.T) {
	req := &pb.StartDeepResearchRequest{
		ProjectId: "project-qa9ei",
		Query:     "nlm cli github.com/tmc",
	}
	want := []interface{}{
		nil,
		[]interface{}{1},
		[]interface{}{"nlm cli github.com/tmc", 1},
		5,
		"project-qa9ei",
	}
	if got := method.EncodeStartDeepResearchArgs(req); !reflect.DeepEqual(got, want) {
		t.Fatalf("generated StartDeepResearch args = %#v, want %#v", got, want)
	}
}
