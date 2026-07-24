package method

import (
	"testing"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/beprotojson"
)

func TestProjectCoverMutationUnknown4RoundTrip(t *testing.T) {
	const wire = `[null,null,null,[null,"title"]]`
	const want = `[null,null,null,[null,"title"],null,null,null,null]`
	var mutation pb.ProjectCoverMutation
	if err := beprotojson.Unmarshal([]byte(wire), &mutation); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	got, err := beprotojson.Marshal(&mutation)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(got) != want {
		t.Fatalf("round trip = %s, want %s", got, want)
	}
}
