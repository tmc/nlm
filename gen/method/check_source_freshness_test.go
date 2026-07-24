package method

import (
	"encoding/json"
	"testing"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

func TestEncodeCheckSourceFreshnessArgs(t *testing.T) {
	req := &pb.CheckSourceFreshnessRequest{
		Source: &pb.SourceIdList{SourceId: "source-id"},
		Context: &pb.RequestContext{
			Version: protoInt32(2),
			Surface: &pb.RequestSurface{Value: protoInt32(1)},
			Caps:    &pb.RequestClientCaps{Version: protoInt32(1), CapabilityCodes: []int32{1, 3}},
		},
	}
	got, err := json.Marshal(EncodeCheckSourceFreshnessArgs(req))
	if err != nil {
		t.Fatal(err)
	}
	const want = `[null,["source-id"],[2,null,[1],[1,null,null,null,null,null,null,null,null,null,[1,3]]]]`
	if string(got) != want {
		t.Fatalf("EncodeCheckSourceFreshnessArgs() = %s, want %s", got, want)
	}
}
