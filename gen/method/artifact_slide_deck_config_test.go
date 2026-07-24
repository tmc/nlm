package method

import (
	"testing"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/beprotojson"
)

func TestArtifactSlideDeckConfigUnknown1RoundTrip(t *testing.T) {
	const wire = `["instructions","en",2,4]`
	var config pb.ArtifactSlideDeckConfig
	if err := beprotojson.Unmarshal([]byte(wire), &config); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	got, err := beprotojson.Marshal(&config)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(got) != wire {
		t.Fatalf("round trip = %s, want %s", got, wire)
	}
}
