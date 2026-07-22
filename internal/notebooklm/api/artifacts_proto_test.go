package api

import (
	"encoding/json"
	"testing"

	method "github.com/tmc/nlm/gen/method"
	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"google.golang.org/protobuf/proto"
)

func TestListArtifactsGeneratedEncoderMatchesCorpusRequest(t *testing.T) {
	req := &pb.ListArtifactsRequest{
		Context: &pb.RequestContext{
			Version:       proto.Int32(2),
			Caps:          &pb.RequestClientCaps{Version: proto.Int32(1), CapabilityCodes: []int32{1}},
			ArtifactTypes: &pb.RequestArtifactTypeFilter{Types: []int32{1, 4, 8, 10, 2, 3, 6, 9}},
		},
		ProjectId: "project-1",
		Filter:    `NOT artifact.status = "ARTIFACT_STATUS_SUGGESTED"`,
	}

	got, err := json.Marshal(method.EncodeListArtifactsArgs(req))
	if err != nil {
		t.Fatalf("marshal generated args: %v", err)
	}
	const want = `[[2,null,null,[1,null,null,null,null,null,null,null,null,null,[1]],[[1,4,8,10,2,3,6,9]]],"project-1","NOT artifact.status = \"ARTIFACT_STATUS_SUGGESTED\""]`
	if string(got) != want {
		t.Fatalf("generated artifact-list args = %s, want %s", got, want)
	}
}

func TestListAudioOverviewRequestUsesCorpusArtifactFilter(t *testing.T) {
	req := &pb.ListArtifactsRequest{Context: universalArtifactRequestContext(), ProjectId: "project-1", Filter: `NOT artifact.status = "ARTIFACT_STATUS_SUGGESTED"`}
	got, err := json.Marshal(method.EncodeListArtifactsArgs(req))
	if err != nil {
		t.Fatal(err)
	}
	want := `[[2,null,null,[1,null,null,null,null,null,null,null,null,null,[1]],[[1,4,8,10,2,3,6,9]]],"project-1","NOT artifact.status = \"ARTIFACT_STATUS_SUGGESTED\""]`
	if string(got) != want {
		t.Fatalf("audio-list args = %s, want %s", got, want)
	}
}
