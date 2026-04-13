package api

import (
	"fmt"
	"strings"
	"testing"

	"github.com/tmc/nlm/internal/batchexecute"
)

func TestParseNotesResponse(t *testing.T) {
	resp := []byte(`[[["note-1",["note-1","hello",[2,"157962509464",[1775436871,282578000]],null,"Test Note"]],["note-2",["note-2","world",[2,"157962509464",[1775436881,282578000]],null,"Second Note"]]],[1775601602,875155000]]`)

	notes, err := parseNotesResponse(resp)
	if err != nil {
		t.Fatalf("parseNotesResponse() error = %v", err)
	}
	if len(notes) != 2 {
		t.Fatalf("len(notes) = %d, want 2", len(notes))
	}

	if got := notes[0].GetNoteId(); got != "note-1" {
		t.Fatalf("notes[0].id = %q, want %q", got, "note-1")
	}
	if got := notes[0].GetTitle(); got != "Test Note" {
		t.Fatalf("notes[0].title = %q, want %q", got, "Test Note")
	}
	if got := notes[1].GetTitle(); got != "Second Note" {
		t.Fatalf("notes[1].title = %q, want %q", got, "Second Note")
	}
}

func TestWrapCreateAudioOverviewErrorAddsGuidance(t *testing.T) {
	err := fmt.Errorf("CreateAudioOverview: %w", &batchexecute.APIError{
		ErrorCode: &batchexecute.ErrorCode{
			Code:      3,
			Type:      batchexecute.ErrorTypeUnavailable,
			Message:   "Service unavailable",
			Retryable: true,
		},
	})

	got := wrapCreateAudioOverviewError(err)
	if !strings.Contains(got.Error(), "enough source text") {
		t.Fatalf("wrapCreateAudioOverviewError() = %q, want guidance about source text", got)
	}
}

func TestParseRenameArtifactResponseAllowsStatusOnlyResponse(t *testing.T) {
	client := &Client{}

	artifact, err := client.parseRenameArtifactResponse([]byte(`[]`), "artifact-1")
	if err != nil {
		t.Fatalf("parseRenameArtifactResponse() error = %v", err)
	}
	if artifact.GetArtifactId() != "artifact-1" {
		t.Fatalf("artifact id = %q, want %q", artifact.GetArtifactId(), "artifact-1")
	}
}

func TestParseArtifactsResponseUsesObservedFieldPositions(t *testing.T) {
	client := &Client{}
	resp := []byte(`[[["artifact-1","Artifact One",3,[[["src-1"]],[["src-2"]]],2],["artifact-2","Artifact Two",8,[[["src-3"]]],7]]]`)

	artifacts, err := client.parseArtifactsResponse(resp)
	if err != nil {
		t.Fatalf("parseArtifactsResponse() error = %v", err)
	}
	if len(artifacts) != 2 {
		t.Fatalf("len(artifacts) = %d, want 2", len(artifacts))
	}

	if got := int32(artifacts[0].GetType()); got != 3 {
		t.Fatalf("artifacts[0].type = %d, want 3", got)
	}
	if got := int32(artifacts[0].GetState()); got != 2 {
		t.Fatalf("artifacts[0].state = %d, want 2", got)
	}
	if got := len(artifacts[0].GetSources()); got != 2 {
		t.Fatalf("artifacts[0].sources = %d, want 2", got)
	}
	if got := artifacts[1].GetSources()[0].GetSourceId().GetSourceId(); got != "src-3" {
		t.Fatalf("artifacts[1].source = %q, want %q", got, "src-3")
	}
}

func TestVideoOverviewResultFromArtifactData(t *testing.T) {
	result := videoOverviewResultFromArtifactData("project-123", []interface{}{
		"video-1",
		"Video Overview",
		float64(3),
		[]interface{}{[]interface{}{[]interface{}{"src-1"}}},
		float64(2),
	})

	if result.VideoID != "video-1" {
		t.Fatalf("VideoID = %q, want %q", result.VideoID, "video-1")
	}
	if result.Title != "Video Overview" {
		t.Fatalf("Title = %q, want %q", result.Title, "Video Overview")
	}
	if !result.IsReady {
		t.Fatal("IsReady = false, want true")
	}
}
