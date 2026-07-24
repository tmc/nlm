package method

import (
	"encoding/json"
	"testing"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

func TestEncodeGetArtifactArgs(t *testing.T) {
	req := &pb.GetArtifactRequest{
		ArtifactId: "artifact-id",
		Context: &pb.RequestContext{
			Version: protoInt32(2),
			Caps:    &pb.RequestClientCaps{Version: protoInt32(1), CapabilityCodes: []int32{1}},
		},
	}
	got, err := json.Marshal(EncodeGetArtifactArgs(req))
	if err != nil {
		t.Fatal(err)
	}
	const want = `["artifact-id",[2,null,null,[1,null,null,null,null,null,null,null,null,null,[1]]]]`
	if string(got) != want {
		t.Fatalf("EncodeGetArtifactArgs() = %s, want %s", got, want)
	}
}
