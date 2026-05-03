package nlmmcp

import (
	"testing"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/notebooklm/api"
)

func TestPaginateDefaultsAndBounds(t *testing.T) {
	items := make([]int, 135)
	for i := range items {
		items[i] = i
	}

	page := paginate(items, 0, -10)
	if page.Limit != defaultPageLimit {
		t.Fatalf("limit = %d, want %d", page.Limit, defaultPageLimit)
	}
	if page.Offset != 0 {
		t.Fatalf("offset = %d, want 0", page.Offset)
	}
	if page.Returned != defaultPageLimit {
		t.Fatalf("returned = %d, want %d", page.Returned, defaultPageLimit)
	}
	if !page.HasMore {
		t.Fatal("has_more = false, want true")
	}
	if page.NextOffset != defaultPageLimit {
		t.Fatalf("next_offset = %d, want %d", page.NextOffset, defaultPageLimit)
	}
}

func TestPaginateCapsLimitAndHandlesPastEndOffset(t *testing.T) {
	items := []string{"a", "b", "c"}

	page := paginate(items, 999, 10)
	if page.Limit != maxPageLimit {
		t.Fatalf("limit = %d, want %d", page.Limit, maxPageLimit)
	}
	if page.Offset != len(items) {
		t.Fatalf("offset = %d, want %d", page.Offset, len(items))
	}
	if page.Returned != 0 {
		t.Fatalf("returned = %d, want 0", page.Returned)
	}
	if page.HasMore {
		t.Fatal("has_more = true, want false")
	}
	if len(page.Items) != 0 {
		t.Fatalf("items len = %d, want 0", len(page.Items))
	}
}

func TestArtifactLabels(t *testing.T) {
	if got := artifactTypeLabel(pb.ArtifactType_ARTIFACT_TYPE_VIDEO_OVERVIEW); got != "ARTIFACT_TYPE_VIDEO_OVERVIEW" {
		t.Fatalf("artifactTypeLabel(video) = %q", got)
	}
	if got := artifactTypeLabel(pb.ArtifactType(8)); got != "ARTIFACT_TYPE_8" {
		t.Fatalf("artifactTypeLabel(8) = %q, want %q", got, "ARTIFACT_TYPE_8")
	}
	if got := artifactStateLabel(pb.ArtifactState(4)); got != "ARTIFACT_STATE_SUGGESTED" {
		t.Fatalf("artifactStateLabel(4) = %q, want %q", got, "ARTIFACT_STATE_SUGGESTED")
	}
	if got := artifactStateLabel(pb.ArtifactState(3)); got != "ARTIFACT_STATE_READY" {
		t.Fatalf("artifactStateLabel(3) = %q, want %q", got, "ARTIFACT_STATE_READY")
	}
	if got := artifactStateLabel(pb.ArtifactState(7)); got != "ARTIFACT_STATE_7" {
		t.Fatalf("artifactStateLabel(7) = %q, want %q", got, "ARTIFACT_STATE_7")
	}
}

func TestArtifactDetailsSummaryFromDetails(t *testing.T) {
	details := &api.ArtifactDetails{
		Artifact: &pb.Artifact{
			ArtifactId: "artifact-1",
			ProjectId:  "notebook-1",
			Type:       pb.ArtifactType_ARTIFACT_TYPE_8,
			State:      pb.ArtifactState(3),
			Sources: []*pb.ArtifactSource{
				{SourceId: &pb.SourceId{SourceId: "source-1"}},
				{SourceId: &pb.SourceId{SourceId: "source-2"}},
			},
		},
		Title:       "Guide to Ensemble Learning",
		Description: "An infographic summary.",
		ImageURL:    "https://lh3.googleusercontent.com/notebooklm/image-token",
		URLs:        []string{"https://lh3.googleusercontent.com/notebooklm/image-token"},
	}

	got := artifactDetailsSummaryFromDetails(details)
	if got.ID != "artifact-1" {
		t.Fatalf("ID = %q, want artifact-1", got.ID)
	}
	if got.ProjectID != "notebook-1" {
		t.Fatalf("ProjectID = %q, want notebook-1", got.ProjectID)
	}
	if got.Type != "ARTIFACT_TYPE_8" {
		t.Fatalf("Type = %q, want ARTIFACT_TYPE_8", got.Type)
	}
	if got.State != "ARTIFACT_STATE_READY" {
		t.Fatalf("State = %q, want ARTIFACT_STATE_READY", got.State)
	}
	if len(got.SourceIDs) != 2 || got.SourceIDs[0] != "source-1" || got.SourceIDs[1] != "source-2" {
		t.Fatalf("SourceIDs = %#v", got.SourceIDs)
	}
	if got.ImageURL == "" || len(got.URLs) != 1 {
		t.Fatalf("URLs not preserved: image=%q urls=%#v", got.ImageURL, got.URLs)
	}
}

func TestArtifactDownloadSummaryFromResult(t *testing.T) {
	got := artifactDownloadSummaryFromResult(&api.ArtifactFilesDownloadResult{
		ArtifactID: "artifact-1",
		OutputPath: "deck-output",
		Files: []*api.ArtifactDownloadResult{
			{Filename: "01-deck.pdf", ContentType: "application/pdf", Bytes: 123},
			{Filename: "02-deck.pptx", ContentType: "application/vnd.openxmlformats-officedocument.presentationml.presentation", Bytes: 456},
		},
	})

	if got.ArtifactID != "artifact-1" {
		t.Fatalf("ArtifactID = %q, want artifact-1", got.ArtifactID)
	}
	if got.OutputPath != "deck-output" {
		t.Fatalf("OutputPath = %q, want deck-output", got.OutputPath)
	}
	if len(got.Files) != 2 {
		t.Fatalf("len(Files) = %d, want 2", len(got.Files))
	}
	if got.Files[0].Filename != "01-deck.pdf" || got.Files[0].Bytes != 123 {
		t.Fatalf("Files[0] = %#v", got.Files[0])
	}
}
