package method

import (
	"testing"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/beprotojson"
)

func TestSourceMetadataPresentZeroRoundTrip(t *testing.T) {
	const wire = `[null,null,null,null,0]`
	const want = `[null,null,null,null,0,null,null,null,null,null,null,null,null,null,null,null,null,null,null,null]`
	var metadata pb.SourceMetadata
	if err := beprotojson.Unmarshal([]byte(wire), &metadata); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	got, err := beprotojson.Marshal(&metadata)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(got) != want {
		t.Fatalf("round trip = %s, want %s", got, want)
	}
}

func TestSourceSettingsDetailUnknown5RoundTrip(t *testing.T) {
	const wire = `[null,null,null,null,[]]`
	const want = `[null,null,null,null,[],null,null]`
	var detail pb.SourceSettingsDetail
	if err := beprotojson.Unmarshal([]byte(wire), &detail); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	got, err := beprotojson.Marshal(&detail)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(got) != want {
		t.Fatalf("round trip = %s, want %s", got, want)
	}
}
