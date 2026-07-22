package api

import (
	"reflect"
	"testing"

	method "github.com/tmc/nlm/gen/method"
	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

func TestStartDeepResearchEncoderCorpusShape(t *testing.T) {
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
