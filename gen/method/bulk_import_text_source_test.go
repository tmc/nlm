package method


import (
	"encoding/json"
	"testing"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/beprotojson"
)

func TestBulkImportTextSourceLinkRoundTrip(t *testing.T) {
	const wire = `[null,null,["https://example.invalid","title"]]`
	var source pb.BulkImportTextSource
	if err := beprotojson.Unmarshal([]byte(wire), &source); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	got, err := beprotojson.Marshal(&source)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var values []interface{}
	if err := json.Unmarshal(got, &values); err != nil {
		t.Fatalf("decode round trip: %v", err)
	}
	if len(values) < 3 || values[2] == nil {
		t.Fatalf("round trip = %s, want field 3 link", got)
	}
}
