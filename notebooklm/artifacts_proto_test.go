package notebooklm

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

func TestAudioOverviewProtoAdapterMatchesLegacyProjection(t *testing.T) {
	raw := []byte(`[[["audio-1","Deep Dive",2,[[["source-1"]]],2]]]`)
	legacy, err := audioOverviewResultsFromArtifacts("project-1", raw)
	if err != nil {
		t.Fatal(err)
	}
	got, err := audioOverviewResultsFromProtoArtifacts("project-1", raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(legacy) != 1 || len(got) != 1 || *legacy[0] != *got[0] {
		t.Fatalf("proto audio projection = %#v, legacy = %#v", got, legacy)
	}
}

func TestArtifactProtoAdapterPreservesRenderedReadyState(t *testing.T) {
	raw := []byte(`[[["slide-1","Deck",8,[[["source-1"]]],3,null,null,null,null,null,null,null,null,null,null,null,null,[["https://contribution.usercontent.google.com/download/slide.pdf"]]]]]`)
	legacy, err := (&Client{}).parseArtifactsResponse(raw)
	if err != nil {
		t.Fatal(err)
	}
	got, err := artifactsFromProtoResponse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(legacy) != 1 || len(got) != 1 {
		t.Fatalf("artifact counts = %d/%d, want 1/1", len(got), len(legacy))
	}
	if got[0].GetState() != pb.ArtifactState_ARTIFACT_STATE_READY || got[0].GetState() != legacy[0].GetState() {
		t.Fatalf("states = %v/%v, want READY", got[0].GetState(), legacy[0].GetState())
	}
}

func TestArtifactProtoAdapterPreservesNoteFields(t *testing.T) {
	raw := []byte(`[[["note-1","Generated note",1,[],2,null,[null,["Summarize",2]]]]]`)
	got, err := artifactsFromProtoResponse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("artifacts = %d, want 1", len(got))
	}
	artifact := got[0]
	if artifact.GetTitle() != "Generated note" {
		t.Fatalf("title = %q, want Generated note", artifact.GetTitle())
	}
	if artifact.GetNote().GetConfig().GetPrompt() != "Summarize" {
		t.Fatalf("prompt = %q, want Summarize", artifact.GetNote().GetConfig().GetPrompt())
	}
}
