package notebooklm

import (
	"encoding/json"
	"testing"

	method "github.com/tmc/nlm/gen/method"
	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"google.golang.org/protobuf/proto"
)

func TestGetAudioFormatsGeneratedEncoderMatchesCapturedSentinel(t *testing.T) {
	req := &pb.GetAudioFormatsRequest{
		Context: &pb.RequestContext{
			Version: proto.Int32(2),
			Caps: &pb.RequestClientCaps{
				Version:         proto.Int32(1),
				CapabilityCodes: []int32{1},
			},
			ArtifactTypes: &pb.RequestArtifactTypeFilter{Types: []int32{1, 4, 8, 10, 2, 3, 6, 9}},
		},
		Mode: proto.Int32(1),
	}

	got, err := json.Marshal(method.EncodeGetAudioFormatsArgs(req))
	if err != nil {
		t.Fatalf("marshal generated args: %v", err)
	}
	const want = `[[2,null,null,[1,null,null,null,null,null,null,null,null,null,[1]],[[1,4,8,10,2,3,6,9]]],null,1]`
	if string(got) != want {
		t.Fatalf("generated audio-formats wire args = %s, want %s", got, want)
	}
}

func TestCreateUniversalArtifactVideoEncoderMatchesCapturedShape(t *testing.T) {
	req := &pb.CreateUniversalArtifactRequest{
		Context: &pb.RequestContext{
			Version:       proto.Int32(2),
			Caps:          &pb.RequestClientCaps{Version: proto.Int32(1), CapabilityCodes: []int32{1}},
			ArtifactTypes: &pb.RequestArtifactTypeFilter{Types: []int32{1, 4, 8, 10, 2, 3, 6, 9}},
		},
		ProjectId: "project-1",
		Options: &pb.UniversalArtifactOptions{
			Kind:         3,
			SourceGroups: []*pb.UniversalArtifactSourceGroup{{Source: &pb.SourceIdList{SourceId: "source-1"}}},
			Video: &pb.UniversalVideoOptions{Details: &pb.UniversalVideoDetails{
				Sources: []*pb.UniversalArtifactSources{{SourceId: "source-1"}},
				Prompt:  proto.String("make a viral instagram reel"),
				Style:   4,
			}},
		},
	}

	got, err := json.Marshal(method.EncodeCreateUniversalArtifactArgs(req))
	if err != nil {
		t.Fatalf("marshal generated args: %v", err)
	}
	const want = `[[2,null,null,[1,null,null,null,null,null,null,null,null,null,[1]],[[1,4,8,10,2,3,6,9]]],"project-1",[null,null,3,[[["source-1"]]],null,null,null,null,[null,null,[[["source-1"]],null,"make a viral instagram reel",null,4]]]]`
	if string(got) != want {
		t.Fatalf("generated video args = %s, want %s", got, want)
	}
}

func TestAudioOverviewResultFromProto(t *testing.T) {
	t.Parallel()

	result := audioOverviewResultFromProto("project-123", &pb.AudioOverview{
		Status:  "READY",
		Content: "Zm9v",
		AudioId: "audio-123",
		Title:   "Overview title",
	})

	if result.ProjectID != "project-123" {
		t.Fatalf("ProjectID = %q, want project-123", result.ProjectID)
	}
	if result.AudioID != "audio-123" {
		t.Fatalf("AudioID = %q, want audio-123", result.AudioID)
	}
	if result.Title != "Overview title" {
		t.Fatalf("Title = %q, want Overview title", result.Title)
	}
	if result.AudioData != "Zm9v" {
		t.Fatalf("AudioData = %q, want Zm9v", result.AudioData)
	}
	if !result.IsReady {
		t.Fatal("IsReady = false, want true")
	}
}

func TestVideoOverviewResultFromProto(t *testing.T) {
	result := videoOverviewResultFromProto("project-123", &pb.Artifact{
		ArtifactId: "video-123",
		Title:      "Video title",
		State:      pb.ArtifactState_ARTIFACT_STATE_READY,
	})
	if result.ProjectID != "project-123" || result.VideoID != "video-123" || result.Title != "Video title" || !result.IsReady {
		t.Fatalf("video result = %+v, want projected ready artifact", result)
	}
}

func TestAudioOverviewResultFromRPC(t *testing.T) {
	t.Parallel()

	result := audioOverviewResultFromRPC("project-123", []interface{}{
		nil,
		nil,
		[]interface{}{float64(3), nil, "audio-123", "Overview title", nil, true, float64(1), nil, "en"},
		nil,
		[]interface{}{false},
	})

	if result.ProjectID != "project-123" {
		t.Fatalf("ProjectID = %q, want project-123", result.ProjectID)
	}
	if result.AudioID != "audio-123" {
		t.Fatalf("AudioID = %q, want audio-123", result.AudioID)
	}
	if result.Title != "Overview title" {
		t.Fatalf("Title = %q, want Overview title", result.Title)
	}
	if !result.IsReady {
		t.Fatal("IsReady = false, want true")
	}
}

func TestAudioOverviewResultsFromArtifacts(t *testing.T) {
	t.Parallel()

	resp := []byte(`[[["audio-2","Newest audio",2,[[["src-1"]]],2],["video-1","Ignore video",3,[[["src-2"]]],2],["audio-1","Older audio",2,[[["src-3"]]],1]]]`)

	results, err := audioOverviewResultsFromArtifacts("project-123", resp)
	if err != nil {
		t.Fatalf("audioOverviewResultsFromArtifacts() error = %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}
	if results[0].AudioID != "audio-2" {
		t.Fatalf("results[0].AudioID = %q, want audio-2", results[0].AudioID)
	}
	if results[0].Title != "Newest audio" {
		t.Fatalf("results[0].Title = %q, want Newest audio", results[0].Title)
	}
	if !results[0].IsReady {
		t.Fatal("results[0].IsReady = false, want true")
	}
	if results[1].AudioID != "audio-1" {
		t.Fatalf("results[1].AudioID = %q, want audio-1", results[1].AudioID)
	}
	if results[1].IsReady {
		t.Fatal("results[1].IsReady = true, want false")
	}
}

func TestMergeAudioOverviewLists(t *testing.T) {
	t.Parallel()

	existing := []*AudioOverviewResult{
		{ProjectID: "project-123", AudioID: "pending-1", Title: "Pending", IsReady: false},
		{ProjectID: "project-123", AudioID: "audio-1"},
	}
	fallback := &AudioOverviewResult{
		ProjectID: "project-123",
		AudioID:   "audio-1",
		Title:     "Ready audio",
		IsReady:   true,
	}
	ready := &AudioOverviewResult{
		ProjectID: "project-123",
		AudioID:   "audio-2",
		Title:     "Second ready",
		IsReady:   true,
	}

	results := mergeAudioOverviewLists(existing, fallback, ready)
	if len(results) != 3 {
		t.Fatalf("len(results) = %d, want 3", len(results))
	}
	if results[1].AudioID != "audio-1" {
		t.Fatalf("results[1].AudioID = %q, want audio-1", results[1].AudioID)
	}
	if results[1].Title != "Ready audio" {
		t.Fatalf("results[1].Title = %q, want Ready audio", results[1].Title)
	}
	if !results[1].IsReady {
		t.Fatal("results[1].IsReady = false, want true")
	}
	if results[2].AudioID != "audio-2" {
		t.Fatalf("results[2].AudioID = %q, want audio-2", results[2].AudioID)
	}
}
