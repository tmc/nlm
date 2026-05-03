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

func TestParseArtifactDetailsResponseExtractsImageURL(t *testing.T) {
	client := &Client{}
	resp := []byte(`[[["artifact-1","Guide to Ensemble Learning",7,[[["src-1"]]],3,null,["payload",["https://lh3.googleusercontent.com/notebooklm/image-token=w2752-d-h1536-mp2"]],"Infographic comparing bagging and boosting."]]]`)

	details, err := client.parseArtifactDetailsResponse(resp, "artifact-1")
	if err != nil {
		t.Fatalf("parseArtifactDetailsResponse() error = %v", err)
	}
	if details.Artifact.GetArtifactId() != "artifact-1" {
		t.Fatalf("artifact id = %q, want artifact-1", details.Artifact.GetArtifactId())
	}
	if details.Title != "Guide to Ensemble Learning" {
		t.Fatalf("Title = %q, want %q", details.Title, "Guide to Ensemble Learning")
	}
	if details.ImageURL != "https://lh3.googleusercontent.com/notebooklm/image-token=w2752-d-h1536-mp2" {
		t.Fatalf("ImageURL = %q", details.ImageURL)
	}
	if details.Description != "Infographic comparing bagging and boosting." {
		t.Fatalf("Description = %q", details.Description)
	}
}

func TestParseArtifactDetailsListResponseExtractsImageURL(t *testing.T) {
	client := &Client{}
	resp := []byte(`[[["artifact-1","Guide to Ensemble Learning",7,[[["src-1"]]],3,null,["payload",["https://lh3.googleusercontent.com/notebooklm/image-token=w2752-d-h1536-mp2"]],"Infographic comparing bagging and boosting."]]]`)

	details, err := client.parseArtifactDetailsListResponse(resp)
	if err != nil {
		t.Fatalf("parseArtifactDetailsListResponse() error = %v", err)
	}
	if len(details) != 1 {
		t.Fatalf("len(details) = %d, want 1", len(details))
	}
	if details[0].ImageURL != "https://lh3.googleusercontent.com/notebooklm/image-token=w2752-d-h1536-mp2" {
		t.Fatalf("ImageURL = %q", details[0].ImageURL)
	}
}

func TestDefaultArtifactImageFilename(t *testing.T) {
	got := defaultArtifactImageFilename(&ArtifactDetails{
		Title: "Guide to Ensemble Learning Techniques!",
	})
	if got != "guide-to-ensemble-learning-techniques.png" {
		t.Fatalf("defaultArtifactImageFilename() = %q", got)
	}
}

func TestArtifactMediaURLsFiltersNotebookLMCDN(t *testing.T) {
	got := artifactMediaURLs(&ArtifactDetails{
		ImageURL: "https://lh3.googleusercontent.com/notebooklm/fallback=w2752",
		URLs: []string{
			"https://example.com/source",
			"https://lh3.googleusercontent.com/notebooklm/image=w2752",
			"https://lh3.googleusercontent.com/notebooklm/image=w2752",
		},
	})
	if len(got) != 1 {
		t.Fatalf("len(artifactMediaURLs) = %d, want 1", len(got))
	}
	if got[0] != "https://lh3.googleusercontent.com/notebooklm/image=w2752" {
		t.Fatalf("artifactMediaURLs()[0] = %q", got[0])
	}
}

func TestArtifactMediaURLsPrefersContributionDownloads(t *testing.T) {
	got := artifactMediaURLs(&ArtifactDetails{
		URLs: []string{
			"https://lh3.googleusercontent.com/notebooklm/preview=w1376",
			"https://contribution.usercontent.google.com/download?filename=Deck.pdf",
			"https://contribution.usercontent.google.com/download?filename=Deck.pptx",
		},
	})
	if len(got) != 2 {
		t.Fatalf("len(artifactMediaURLs) = %d, want 2", len(got))
	}
	if got[0] != "https://contribution.usercontent.google.com/download?filename=Deck.pdf" {
		t.Fatalf("artifactMediaURLs()[0] = %q", got[0])
	}
	if got[1] != "https://contribution.usercontent.google.com/download?filename=Deck.pptx" {
		t.Fatalf("artifactMediaURLs()[1] = %q", got[1])
	}
}

func TestDefaultArtifactFilenameUsesURLFilename(t *testing.T) {
	got := defaultArtifactFilename(&ArtifactDetails{Title: "Ignored"}, "https://contribution.usercontent.google.com/download?filename=The_Deck.pptx", "application/octet-stream")
	if got != "The_Deck.pptx" {
		t.Fatalf("defaultArtifactFilename() = %q", got)
	}
}

func TestMediaURLWithAuthUser(t *testing.T) {
	client := &Client{}
	got := client.mediaURLWithAuthUser("https://contribution.usercontent.google.com/download?filename=The_Deck.pptx")
	if got != "https://contribution.usercontent.google.com/download?authuser=0&filename=The_Deck.pptx" {
		t.Fatalf("mediaURLWithAuthUser() = %q", got)
	}

	client.SetAuthUser("2")
	got = client.mediaURLWithAuthUser("https://contribution.usercontent.google.com/download?authuser=1&filename=The_Deck.pptx")
	if got != "https://contribution.usercontent.google.com/download?authuser=1&filename=The_Deck.pptx" {
		t.Fatalf("mediaURLWithAuthUser(existing) = %q", got)
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
