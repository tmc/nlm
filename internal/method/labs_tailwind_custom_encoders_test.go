package method

import (
	"encoding/json"
	"testing"

	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

func TestEncodePublishGuidebookArgsV2(t *testing.T) {
	req := &notebooklmv1alpha1.PublishGuidebookRequest{
		GuidebookId: "00000000-0000-4000-8000-000000000301",
	}
	got := EncodePublishGuidebookArgsV2(req)
	gotJSON := mustJSON(t, got)

	// HAR fixture: [[], null, null, "00000000-0000-4000-8000-000000000301", 20]
	want := `[[],null,null,"00000000-0000-4000-8000-000000000301",20]`
	if gotJSON != want {
		t.Errorf("EncodePublishGuidebookArgsV2:\n got: %s\nwant: %s", gotJSON, want)
	}
}

func TestEncodeShareGuidebookArgsV2(t *testing.T) {
	req := &notebooklmv1alpha1.ShareGuidebookRequest{
		GuidebookId: "test-guidebook",
	}
	got := EncodeShareGuidebookArgsV2(req)
	gotJSON := mustJSON(t, got)

	// HAR fixture: [[2,null,null,[1,null,null,null,null,null,null,null,null,null,[1]],[[1,4,2,3,6,5]]],null,1]
	want := `[[2,null,null,[1,null,null,null,null,null,null,null,null,null,[1]],[[1,4,2,3,6,5]]],null,1]`
	if gotJSON != want {
		t.Errorf("EncodeShareGuidebookArgsV2:\n got: %s\nwant: %s", gotJSON, want)
	}
}

func TestEncodeDeleteGuidebookArgsV2(t *testing.T) {
	req := &notebooklmv1alpha1.DeleteGuidebookRequest{
		GuidebookId: "1",
	}
	got := EncodeDeleteGuidebookArgsV2(req)
	gotJSON := mustJSON(t, got)

	// HAR fixture structure: [[[[null,"1",null],[null,null,null,null,null,null,null,null,null,[null,null,2]],1]]]
	// The version field is nil (omitted), but structure must match.
	want := `[[[[null,"1",null],[null,null,null,null,null,null,null,null,null,[null,null,2]],1]]]`
	if gotJSON != want {
		t.Errorf("EncodeDeleteGuidebookArgsV2:\n got: %s\nwant: %s", gotJSON, want)
	}
}

func TestEncodeGuidebookGenerateAnswerArgsV2(t *testing.T) {
	req := &notebooklmv1alpha1.GuidebookGenerateAnswerRequest{
		GuidebookId: "00000000-0000-4000-8000-000000000302",
		Question:    "What is the API?",
	}
	got := EncodeGuidebookGenerateAnswerArgsV2(req)
	gotJSON := mustJSON(t, got)

	want := `["What is the API?","00000000-0000-4000-8000-000000000302",0,""]`
	if gotJSON != want {
		t.Errorf("EncodeGuidebookGenerateAnswerArgsV2:\n got: %s\nwant: %s", gotJSON, want)
	}
}

func TestEncodeDeleteArtifactArgsV2(t *testing.T) {
	req := &notebooklmv1alpha1.DeleteArtifactRequest{
		ArtifactId: "test-artifact-id",
	}
	got := EncodeDeleteArtifactArgsV2(req)
	gotJSON := mustJSON(t, got)

	want := `["test-artifact-id",[2]]`
	if gotJSON != want {
		t.Errorf("EncodeDeleteArtifactArgsV2:\n got: %s\nwant: %s", gotJSON, want)
	}
}

func TestEncodeDeleteAudioOverviewArgsV2(t *testing.T) {
	req := &notebooklmv1alpha1.DeleteAudioOverviewRequest{
		ProjectId: "00000000-0000-4000-8000-000000000004",
	}
	got := EncodeDeleteAudioOverviewArgsV2(req)
	gotJSON := mustJSON(t, got)

	// HAR fixture: [["00000000-0000-4000-8000-000000000004"],[2],[2]]
	want := `[["00000000-0000-4000-8000-000000000004"],[2],[2]]`
	if gotJSON != want {
		t.Errorf("EncodeDeleteAudioOverviewArgsV2:\n got: %s\nwant: %s", gotJSON, want)
	}
}

func TestEncodeShareAudioArgsV2(t *testing.T) {
	req := &notebooklmv1alpha1.ShareAudioRequest{
		ProjectId: "00000000-0000-4000-8000-000000000005",
	}
	got := EncodeShareAudioArgsV2(req)
	gotJSON := mustJSON(t, got)

	// HAR fixture: [[],null,"00000000-0000-4000-8000-000000000005",20]
	want := `[[],null,"00000000-0000-4000-8000-000000000005",20]`
	if gotJSON != want {
		t.Errorf("EncodeShareAudioArgsV2:\n got: %s\nwant: %s", gotJSON, want)
	}
}

func TestEncodeGetProjectAnalyticsArgsV2(t *testing.T) {
	req := &notebooklmv1alpha1.GetProjectAnalyticsRequest{
		ProjectId: "00000000-0000-4000-8000-000000000005",
	}
	got := EncodeGetProjectAnalyticsArgsV2(req)

	// Verify structure: ["<project_id>", null, [<int>, <int>], [2]]
	if len(got) != 4 {
		t.Fatalf("expected 4 elements, got %d", len(got))
	}
	if got[0] != "00000000-0000-4000-8000-000000000005" {
		t.Errorf("field 0: got %v, want project ID", got[0])
	}
	if got[1] != nil {
		t.Errorf("field 1: got %v, want nil", got[1])
	}
	ts, ok := got[2].([]interface{})
	if !ok || len(ts) != 2 {
		t.Fatalf("field 2: expected [seconds, nanos], got %v", got[2])
	}
	if _, ok := ts[0].(int64); !ok {
		t.Errorf("field 2[0]: expected int64 timestamp seconds, got %T", ts[0])
	}
	if _, ok := ts[1].(int64); !ok {
		t.Errorf("field 2[1]: expected int64 timestamp nanos, got %T", ts[1])
	}
	ctx, ok := got[3].([]interface{})
	if !ok || len(ctx) != 1 || ctx[0] != 2 {
		t.Errorf("field 3: got %v, want [2]", got[3])
	}
}

func mustJSON(t *testing.T, v interface{}) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	return string(b)
}
