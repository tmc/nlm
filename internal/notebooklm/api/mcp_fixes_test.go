package api

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/batchexecute"
	"github.com/tmc/nlm/internal/beprotojson"
)

func TestNotesFromWireResponse(t *testing.T) {
	resp := []byte(`[[["note-1",["note-1","hello",[2,"157962509464",[1775436871,282578000]],null,"Test Note"]],["note-2",["note-2","world",[2,"157962509464",[1775436881,282578000]],null,"Second Note"]]],[1775601602,875155000]]`)
	var wire pb.GetNotesWireResponse
	if err := beprotojson.Unmarshal(resp, &wire); err != nil {
		t.Fatalf("decode notes: %v", err)
	}
	notes := notesFromWireResponse(&wire)
	if len(notes) != 2 {
		t.Fatalf("len(notes) = %d, want 2", len(notes))
	}

	if got := notes[0].GetNoteId(); got != "note-1" {
		t.Fatalf("notes[0].id = %q, want %q", got, "note-1")
	}
	if got := notes[0].GetTitle(); got != "Test Note" {
		t.Fatalf("notes[0].title = %q, want %q", got, "Test Note")
	}
	if got := notes[0].GetContentText(); got != "hello" {
		t.Fatalf("notes[0].content = %q, want %q", got, "hello")
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

// TestParseArtifactCompletedDeckNotFailed locks in the fix for slide decks
// being mislabeled FAILED. A fully-rendered deck's payload holds 3 at the
// state position [4] — the same value our enum assigns to FAILED — yet carries
// .pdf/.pptx download URLs proving it is done. The parser must trust the output
// URLs and report READY. Fixture is a real v9rmvd slide-deck payload.
func TestParseArtifactCompletedDeckNotFailed(t *testing.T) {
	client := &Client{}
	raw := mustReadAPIFixture(t, "testdata/v9rmvd_slide_artifact.json")

	var artifactData interface{}
	if err := json.Unmarshal(raw, &artifactData); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}

	// Sanity: the raw state int really is 3 (would map to FAILED) before the fix.
	arr := artifactData.([]interface{})
	if got, _ := int32At(arr, 4); got != 3 {
		t.Fatalf("fixture state[4] = %d, want 3 (the FAILED-valued code on a completed deck)", got)
	}

	artifact := client.parseArtifactFromResponse(artifactData)
	if artifact == nil {
		t.Fatal("parseArtifactFromResponse returned nil")
	}
	if artifact.GetState() != pb.ArtifactState_ARTIFACT_STATE_READY {
		t.Fatalf("state = %s (%d), want ARTIFACT_STATE_READY — completed deck with download URLs",
			artifact.GetState(), int32(artifact.GetState()))
	}

	urls := extractArtifactDownloadURLs(artifactData)
	if len(urls) != 2 {
		t.Fatalf("download URLs = %d, want 2 (.pdf and .pptx): %v", len(urls), urls)
	}
	var pdf, pptx bool
	for _, u := range urls {
		if !strings.HasPrefix(u, artifactDownloadURLPrefix) {
			t.Errorf("download URL missing expected prefix: %s", u)
		}
		if strings.Contains(u, ".pdf") {
			pdf = true
		}
		if strings.Contains(u, ".pptx") {
			pptx = true
		}
	}
	if !pdf || !pptx {
		t.Fatalf("expected both .pdf and .pptx URLs; got pdf=%v pptx=%v", pdf, pptx)
	}
}

// TestExtractArtifactDownloadURLsNoFalsePositive verifies that an artifact
// without rendered output yields no URLs and keeps its raw state.
func TestExtractArtifactDownloadURLsNoFalsePositive(t *testing.T) {
	client := &Client{}
	// A still-generating deck: state 1 (CREATING), no download URLs anywhere.
	resp := []byte(`[["artifact-x","Pending Deck",8,[[["src-1"]]],1,null,"https://example.com/not-a-download"]]`)
	var data []interface{}
	if err := json.Unmarshal(resp, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	artifact := client.parseArtifactFromResponse(data[0])
	if artifact.GetState() != pb.ArtifactState_ARTIFACT_STATE_CREATING {
		t.Fatalf("state = %s, want CREATING (no download URLs, must not be overridden)", artifact.GetState())
	}
	if urls := extractArtifactDownloadURLs(data[0]); len(urls) != 0 {
		t.Fatalf("expected no download URLs, got %v", urls)
	}
}

func mustReadAPIFixture(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}
	return b
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
