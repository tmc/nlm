package api

import (
	"encoding/json"
	"testing"
)

// TestBulkImportEncoderShape verifies the 5-position argument layout for
// LBwxtb BulkImportFromResearch. The RPC is polymorphic with
// DeleteDeepResearch: delete has 4 positions, bulk-import adds the
// 5th (sources array). The server discriminates on arg-4 presence.
func TestBulkImportEncoderShape(t *testing.T) {
	sources := []BulkImportSource{
		{URL: "https://en.wikipedia.org/wiki/HAR_(file_format)", Title: "HAR (file format) - Wikipedia"},
		{URL: "https://w3c.github.io/web-performance/specs/HAR/Overview.html", Title: "HTTP Archive (HAR) format - W3C on GitHub"},
		{URL: "https://github.com/google/har2csv", Title: "google/har2csv: A simple NodeJS CLI tool"},
	}
	got := encodeBulkImportArgsJSON(t, "00000000-0000-4000-8000-000000000401", "00000000-0000-4000-8000-000000000006", sources)
	want := canonicalJSON(t, loadFixture(t, "LBwxtb_bulk_import_request.json"))
	if got != want {
		t.Errorf("encoder shape mismatch\n got: %s\nwant: %s", got, want)
	}
}

// TestDeleteDeepResearchEncoderShape locks the 4-position delete shape
// so a future refactor of BulkImportFromResearch cannot accidentally
// make delete emit the 5-position bulk shape.
func TestDeleteDeepResearchEncoderShape(t *testing.T) {
	got := encodeDeleteDeepResearchArgsJSON(t, "00000000-0000-4000-8000-000000000402", "00000000-0000-4000-8000-000000000006")
	want := canonicalJSON(t, loadFixture(t, "LBwxtb_delete_request.json"))
	if got != want {
		t.Errorf("encoder shape mismatch\n got: %s\nwant: %s", got, want)
	}
}

// TestParseBulkImportResponse decodes the rich source-metadata response
// into the minimal fields the CLI surfaces: source_id, title, URL.
func TestParseBulkImportResponse(t *testing.T) {
	result, err := parseBulkImportResponse(loadFixture(t, "LBwxtb_bulk_import_response.json"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(result) != 3 {
		t.Fatalf("got %d imported sources, want 3", len(result))
	}
	first := result[0]
	if first.SourceID != "00000000-0000-4000-8000-000000000106" {
		t.Errorf("SourceID: got %q", first.SourceID)
	}
	if first.Title != "HAR (file format) - Wikipedia" {
		t.Errorf("Title: got %q", first.Title)
	}
	if first.URL != "https://en.wikipedia.org/wiki/HAR_(file_format)" {
		t.Errorf("URL: got %q", first.URL)
	}
}

func encodeBulkImportArgsJSON(t *testing.T, conv, proj string, sources []BulkImportSource) string {
	t.Helper()
	args := bulkImportArgs(conv, proj, sources)
	buf, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(buf)
}

func encodeDeleteDeepResearchArgsJSON(t *testing.T, conv, proj string) string {
	t.Helper()
	args := deleteDeepResearchArgs(conv, proj)
	buf, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(buf)
}
