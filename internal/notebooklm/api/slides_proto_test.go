package api

import (
	"encoding/json"
	"testing"

	"github.com/tmc/nlm/gen/method"
	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

func TestCreateSlideDeckRequestEncoderMatchesCorpus(t *testing.T) {
	req := &pb.CreateUniversalArtifactRequest{
		Context:   universalArtifactRequestContext(),
		ProjectId: "project-1",
		Options: &pb.UniversalArtifactOptions{
			Kind:         8,
			SourceGroups: universalArtifactSourceGroups([]string{"source-1"}),
			Slides:       []*pb.UniversalSlideOptions{{Language: "en", Format: 2, Style: 4}},
		},
	}
	got, err := json.Marshal(method.EncodeCreateUniversalArtifactArgs(req))
	if err != nil {
		t.Fatal(err)
	}
	want := `[[2,null,null,[1,null,null,null,null,null,null,null,null,null,[1]],[[1,4,8,10,2,3,6,9]]],"project-1",[null,null,8,[[["source-1"]]],null,null,null,null,null,null,null,null,null,null,null,null,[[null,"en",2,4]]]]`
	if string(got) != want {
		t.Fatalf("encoded slide request = %s, want %s", got, want)
	}
}
