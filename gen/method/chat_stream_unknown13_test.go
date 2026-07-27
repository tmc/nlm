package method

import (
	"testing"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/beprotojson"
)

func TestChatStreamUnknown13RoundTrip(t *testing.T) {
	const wire = `[[["source-id",["",0,0]]]]`
	var value pb.ChatStreamUnknown13
	if err := beprotojson.Unmarshal([]byte(wire), &value); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	got, err := beprotojson.Marshal(&value)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(got) != wire {
		t.Fatalf("round trip = %s, want %s", got, wire)
	}
}
