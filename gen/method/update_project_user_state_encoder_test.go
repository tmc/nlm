package method

import (
	"encoding/json"
	"testing"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"google.golang.org/protobuf/proto"
)

func TestEncodeUpdateProjectUserStatePinnedArgs(t *testing.T) {
	got, err := json.Marshal(EncodeUpdateProjectUserStateArgs(&pb.UpdateProjectUserStateRequest{
		ProjectId: "project-id",
		Value: &pb.UpdateProjectUserStateValue{Detail: &pb.UpdateProjectUserStateValueInner{
			Pinned: proto.Int32(1),
		}},
		Keys: &pb.UpdateProjectUserStateKeys{Keys: []string{"state-key"}},
	}))
	if err != nil {
		t.Fatal(err)
	}
	const want = `[null,"project-id",[null,[1]],[["state-key"]]]`
	if string(got) != want {
		t.Fatalf("EncodeUpdateProjectUserStateArgs = %s, want %s", got, want)
	}
}
