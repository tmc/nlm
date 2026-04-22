package api

import "testing"

func TestParseProjectDetailsResponse(t *testing.T) {
	t.Parallel()

	resp := []byte(`[[["owner@example.com",1,[],["Travis Cline","https://example.com/avatar.png"]]],[true,true],1000,true]`)
	details, err := parseProjectDetailsResponse(resp)
	if err != nil {
		t.Fatalf("parseProjectDetailsResponse() error = %v", err)
	}
	if details.OwnerName != "Travis Cline" {
		t.Fatalf("OwnerName = %q, want %q", details.OwnerName, "Travis Cline")
	}
	if !details.IsPublic {
		t.Fatal("IsPublic = false, want true")
	}
}

func TestParseShareProjectResponseFallsBackToNotebookURL(t *testing.T) {
	t.Parallel()

	resp := []byte(`[]`)
	result, err := parseShareProjectResponse("notebook-123", true, resp)
	if err != nil {
		t.Fatalf("parseShareProjectResponse() error = %v", err)
	}
	if result.ShareUrl != "https://notebooklm.google.com/notebook/notebook-123" {
		t.Fatalf("ShareUrl = %q, want notebook URL fallback", result.ShareUrl)
	}
	if result.Settings == nil || !result.Settings.IsPublic {
		t.Fatal("Settings.IsPublic = false, want true")
	}
}

func TestParseShareProjectResponsePrivateEmptyPayload(t *testing.T) {
	t.Parallel()

	resp := []byte(`[]`)
	result, err := parseShareProjectResponse("notebook-123", false, resp)
	if err != nil {
		t.Fatalf("parseShareProjectResponse() error = %v", err)
	}
	if result.ShareUrl != "" {
		t.Fatalf("ShareUrl = %q, want empty for private share without explicit URL", result.ShareUrl)
	}
	if result.Settings == nil || result.Settings.IsPublic {
		t.Fatal("Settings.IsPublic = true, want false")
	}
}

func TestParseShareProjectResponseExtractsShareID(t *testing.T) {
	t.Parallel()

	resp := []byte(`["123e4567-e89b-12d3-a456-426614174000"]`)
	result, err := parseShareProjectResponse("notebook-123", false, resp)
	if err != nil {
		t.Fatalf("parseShareProjectResponse() error = %v", err)
	}
	if result.ShareId != "123e4567-e89b-12d3-a456-426614174000" {
		t.Fatalf("ShareId = %q, want extracted UUID", result.ShareId)
	}
}
