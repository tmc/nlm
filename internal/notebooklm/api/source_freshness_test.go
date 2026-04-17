package api

import (
	"encoding/json"
	"testing"
	"time"

	intmethod "github.com/tmc/nlm/gen/method"
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

func encodeFreshnessRequestJSON(t *testing.T, sourceID string) string {
	t.Helper()
	args := intmethod.EncodeCheckSourceFreshnessArgs(&notebooklmv1alpha1.CheckSourceFreshnessRequest{SourceId: sourceID})
	buf, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("marshal encoder args: %v", err)
	}
	return string(buf)
}

func encodeRefreshRequestJSON(t *testing.T, sourceID string) string {
	t.Helper()
	args := intmethod.EncodeRefreshSourceArgs(&notebooklmv1alpha1.RefreshSourceRequest{SourceId: sourceID})
	buf, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("marshal encoder args: %v", err)
	}
	return string(buf)
}

// TestParseCheckFreshnessResponse_Stale locks the yR9Yof response shape
// [null, is_fresh_bool, [source_id]] against a HAR-verified stale sample.
func TestParseCheckFreshnessResponse_Stale(t *testing.T) {
	result, err := parseCheckFreshnessResponse(loadFixture(t, "yR9Yof_check_freshness_response_stale.json"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result.IsFresh {
		t.Errorf("IsFresh: want false (stale fixture)")
	}
	if result.SourceID != "decafbad-0000-0000-0000-000000000001" {
		t.Errorf("SourceID: got %q", result.SourceID)
	}
}

// TestParseCheckFreshnessResponse_Fresh exercises the speculative
// [null, true, [source_id]] shape. The fresh fixture was synthesized
// because the HAR capture did not contain a fresh-state response (the
// post-sync flow re-fires hizoJc+tr032e instead of yR9Yof). If a future
// capture contradicts this shape, update the fixture and this test.
func TestParseCheckFreshnessResponse_Fresh(t *testing.T) {
	result, err := parseCheckFreshnessResponse(loadFixture(t, "yR9Yof_check_freshness_response_fresh.json"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !result.IsFresh {
		t.Errorf("IsFresh: want true (fresh fixture)")
	}
	if result.SourceID != "decafbad-0000-0000-0000-000000000001" {
		t.Errorf("SourceID: got %q", result.SourceID)
	}
}

// TestParseRefreshSourceResponse_Drive locks the rich FLmJqe response
// shape captured from a real Google-Drive sync.
func TestParseRefreshSourceResponse_Drive(t *testing.T) {
	result, err := parseRefreshSourceResponse(loadFixture(t, "FLmJqe_refresh_source_response_drive.json"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if result.SourceID != "decafbad-0000-0000-0000-000000000001" {
		t.Errorf("SourceID: got %q", result.SourceID)
	}
	if result.Title != "synthetic Drive doc fixture" {
		t.Errorf("Title: got %q", result.Title)
	}
	if result.DriveFileID != "1synthetic_drive_file_id_for_test_fixture000" {
		t.Errorf("DriveFileID: got %q", result.DriveFileID)
	}
	if result.SourceRevisionID != "decafbad-0000-0000-0000-000000000002" {
		t.Errorf("SourceRevisionID: got %q", result.SourceRevisionID)
	}
	if result.FinalState != 2 {
		t.Errorf("FinalState: got %d, want 2", result.FinalState)
	}
	wantRevTime := time.Unix(1776454825, 528965000).UTC()
	if !result.RevisionTime.Equal(wantRevTime) {
		t.Errorf("RevisionTime: got %v, want %v", result.RevisionTime, wantRevTime)
	}
}

// TestCheckFreshnessEncoderShape verifies the argbuilder produces the
// HAR-verified [null, [source_id], [2]] shape. Before the 2026-04-17 fix
// the encoder emitted [null, [source_id], [4]] which the server silently
// accepted for non-Drive sources while rejecting Drive sources outright
// with "One or more arguments are invalid" (commit ab27baa retracted).
func TestCheckFreshnessEncoderShape(t *testing.T) {
	want := canonicalJSON(t, loadFixture(t, "yR9Yof_check_freshness_request.json"))
	got := encodeFreshnessRequestJSON(t, "decafbad-0000-0000-0000-000000000001")
	if got != want {
		t.Errorf("encoder shape mismatch\n got: %s\nwant: %s", got, want)
	}
}

func TestRefreshSourceEncoderShape(t *testing.T) {
	want := canonicalJSON(t, loadFixture(t, "FLmJqe_refresh_source_request.json"))
	got := encodeRefreshRequestJSON(t, "decafbad-0000-0000-0000-000000000001")
	if got != want {
		t.Errorf("encoder shape mismatch\n got: %s\nwant: %s", got, want)
	}
}

// canonicalJSON re-marshals raw JSON through the stdlib encoder so
// comparisons are insensitive to whitespace in the fixture source file.
func canonicalJSON(t *testing.T, raw []byte) string {
	t.Helper()
	var v interface{}
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("canonicalize fixture: %v", err)
	}
	buf, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("canonicalize fixture marshal: %v", err)
	}
	return string(buf)
}
