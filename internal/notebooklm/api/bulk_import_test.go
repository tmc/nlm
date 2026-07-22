package api

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	method "github.com/tmc/nlm/gen/method"
	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/batchexecute"
	"github.com/tmc/nlm/internal/beprotojson"
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
	conversationID := "00000000-0000-4000-8000-000000000402"
	projectID := "00000000-0000-4000-8000-000000000006"
	legacy := encodeDeleteDeepResearchArgsJSON(t, conversationID, projectID)
	encoded := method.EncodeDeleteDeepResearchArgs(&pb.DeleteDeepResearchRequest{
		ProjectId:      projectID,
		ConversationId: conversationID,
	})
	encodedJSON, err := json.Marshal(encoded)
	if err != nil {
		t.Fatalf("marshal generated encoder: %v", err)
	}
	want := canonicalJSON(t, loadFixture(t, "LBwxtb_delete_request.json"))
	if legacy != want || string(encodedJSON) != want {
		t.Errorf("encoder shape mismatch\n legacy: %s\n generated: %s\nwant: %s", legacy, encodedJSON, want)
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

func TestBulkImportProtoAdapterMatchesLegacyParser(t *testing.T) {
	raw := loadFixture(t, "LBwxtb_bulk_import_response.json")
	legacy, err := parseBulkImportResponse(raw)
	if err != nil {
		t.Fatalf("legacy parser: %v", err)
	}
	var wire pb.BulkImportFromResearchResponse
	if err := beprotojson.Unmarshal(raw, &wire); err != nil {
		t.Fatalf("proto decoder: %v", err)
	}
	assertEquivalent(t, "bulk import adaptation", legacy, bulkImportResultsFromProto(&wire))
}

// TestBulkImportTextSourceCorpusProjection records the note/text LBwxtb
// variant. Its extra result wrapper made the old URL-only parser return no
// results, while the generated Source model recovers the imported source.
func TestBulkImportTextSourceCorpusProjection(t *testing.T) {
	var files []string
	err := filepath.WalkDir("/tmp/nlm-traffic", func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() && entry.Name() == "notebooklm.google.com.jsonl" {
			files = append(files, path)
		}
		return nil
	})
	if err != nil || len(files) == 0 {
		t.Skip("/tmp/nlm-traffic corpus is not available")
	}

	variants := 0
	for _, file := range files {
		f, err := os.Open(file)
		if err != nil {
			t.Fatal(err)
		}
		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 64*1024), 64*1024*1024)
		for record := 1; scanner.Scan(); record++ {
			var entry struct {
				Request struct {
					URL      string                `json:"url"`
					PostData struct{ Text string } `json:"postData"`
				} `json:"request"`
				Response struct {
					Content struct{ Text, Encoding string } `json:"content"`
				} `json:"response"`
			}
			if json.Unmarshal(scanner.Bytes(), &entry) != nil || !strings.Contains(entry.Request.URL, "rpcids=LBwxtb") {
				continue
			}
			requestBody, err := base64.StdEncoding.DecodeString(entry.Request.PostData.Text)
			if err != nil {
				continue
			}
			request, err := batchexecute.DecodeRequest(string(requestBody))
			if err != nil || len(request.RPCs) != 1 || request.RPCs[0].ID != "LBwxtb" {
				continue
			}
			var args []interface{}
			if json.Unmarshal(request.RPCs[0].Args, &args) != nil || len(args) != 5 {
				continue
			}
			context, isTextSource := args[0].([]interface{})
			sources, sourcesOK := args[4].([]interface{})
			if !isTextSource || !sourcesOK || len(sources) != 1 {
				continue
			}
			source, sourceOK := sources[0].([]interface{})
			if !sourceOK || len(source) < 2 {
				continue
			}
			if _, ok := source[1].([]interface{}); !ok {
				continue
			}

			responseBody := entry.Response.Content.Text
			if entry.Response.Content.Encoding == "base64" {
				decoded, err := base64.StdEncoding.DecodeString(responseBody)
				if err != nil {
					t.Fatalf("%s:%d: base64 response: %v", file, record, err)
				}
				responseBody = string(decoded)
			}
			response, err := batchexecute.DecodeResponse(responseBody)
			if err != nil || len(response.Responses) != 1 || response.Responses[0].ID != "LBwxtb" {
				continue
			}
			legacy, err := parseBulkImportResponse(response.Responses[0].Data)
			if err != nil {
				t.Fatalf("%s:%d: legacy parse: %v", file, record, err)
			}
			var wire pb.BulkImportFromResearchResponse
			if err := beprotojson.Unmarshal(response.Responses[0].Data, &wire); err != nil {
				t.Fatalf("%s:%d: proto decode: %v", file, record, err)
			}
			got := bulkImportResultsFromProto(&wire)
			if len(legacy) != 0 || len(got) != 1 || got[0].SourceID == "" || got[0].Title == "" || got[0].URL != "" {
				t.Fatalf("%s:%d: legacy=%+v generated=%+v", file, record, legacy, got)
			}
			_ = context // the non-nil context distinguishes the text wire variant.
			variants++
		}
		if err := scanner.Err(); err != nil {
			t.Fatal(err)
		}
		f.Close()
	}
	if variants < 1 {
		t.Fatalf("LBwxtb note/text variants=%d, want at least 1", variants)
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
