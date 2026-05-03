package nlmmcp

import (
	"testing"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
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
