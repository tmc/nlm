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
