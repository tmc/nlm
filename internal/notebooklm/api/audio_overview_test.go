package api

import (
	"testing"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

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
