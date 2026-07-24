package method

import (
	"encoding/json"
	"testing"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

func TestEncodeGenerateMagicViewArgs(t *testing.T) {
	req := &pb.GenerateMagicViewRequest{
		Context: &pb.MagicViewRequestContext{
			Version: protoInt32(2),
			Surface: &pb.MagicViewRequestSurface{Value: protoInt32(1)},
			Caps:    &pb.MagicViewRequestCaps{Version: protoInt32(1), CapabilityCodes: []int32{1, 3}},
		},
		ProjectId: "project-id",
	}
	got, err := json.Marshal(EncodeGenerateMagicViewArgs(req))
	if err != nil {
		t.Fatal(err)
	}
	const want = `[[2,null,[1],[1,null,null,null,null,null,null,null,null,null,[1,3]]],"project-id"]`
	if string(got) != want {
		t.Fatalf("EncodeGenerateMagicViewArgs() = %s, want %s", got, want)
	}
}
