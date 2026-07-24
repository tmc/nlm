package method

import (
	"encoding/json"
	"testing"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/beprotojson"
)

func TestEncodeListExpertIntelligenceContentArgs(t *testing.T) {
	req := &pb.ListExpertIntelligenceContentRequest{
		Context: &pb.RequestContext{
			Version: protoInt32(2),
			Surface: &pb.RequestSurface{Value: protoInt32(1)},
			Caps:    &pb.RequestClientCaps{Version: protoInt32(1), CapabilityCodes: []int32{1, 3}},
		},
		ContentKind: 1,
	}
	got, err := json.Marshal(EncodeListExpertIntelligenceContentArgs(req))
	if err != nil {
		t.Fatal(err)
	}
	const want = `[[2,null,[1],[1,null,null,null,null,null,null,null,null,null,[1,3]]],1]`
	if string(got) != want {
		t.Fatalf("EncodeListExpertIntelligenceContentArgs() = %s, want %s", got, want)
	}
}

func TestExpertIntelligenceItemRoundTrip(t *testing.T) {
	const wire = `[[["book-id",1,"Title","Description","https://example.invalid/cover",true,3,["Author"],4.25,[1234567890]]],null]`
	var response pb.ListExpertIntelligenceContentResponse
	if err := beprotojson.Unmarshal([]byte(wire), &response); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	got, err := beprotojson.Marshal(&response)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(got) != wire {
		t.Fatalf("round trip = %s, want %s", got, wire)
	}
}
