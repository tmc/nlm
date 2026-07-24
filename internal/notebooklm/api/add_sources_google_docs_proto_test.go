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

func TestAddSourcesGoogleDocsCorpusProjection(t *testing.T) {
	path := filepath.Join("/tmp", "nlm-traffic", "new-rec3", "notebooklm.google.com", "notebooklm.google.com.jsonl")
	f, err := os.Open(path)
	if err != nil {
		t.Skipf("open live Google Docs capture: %v", err)
	}
	defer f.Close()

	variants := 0
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 64*1024*1024)
	for record := 1; scanner.Scan(); record++ {
		var entry struct {
			Request struct {
				URL      string `json:"url"`
				PostData struct {
					Text string `json:"text"`
				} `json:"postData"`
			} `json:"request"`
			Response struct {
				Content struct {
					Text     string `json:"text"`
					Encoding string `json:"encoding"`
				} `json:"content"`
			} `json:"response"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil || !strings.Contains(entry.Request.URL, "rpcids=izAoDd") {
			continue
		}
		body, err := base64.StdEncoding.DecodeString(entry.Request.PostData.Text)
		if err != nil {
			t.Fatalf("%s:%d: base64 request: %v", path, record, err)
		}
		request, err := batchexecute.DecodeRequest(string(body))
		if err != nil {
			t.Fatalf("%s:%d: decode request: %v", path, record, err)
		}
		var rawRequest json.RawMessage
		for _, call := range request.RPCs {
			if call.ID == "izAoDd" {
				rawRequest = call.Args
				break
			}
		}
		if !strings.Contains(string(rawRequest), `"application/vnd.google-apps.document"`) {
			continue
		}
		var generatedRequest pb.AddSourceRequest
		if err := beprotojson.Unmarshal(rawRequest, &generatedRequest); err != nil {
			t.Fatalf("%s:%d: generated request decode: %v", path, record, err)
		}
		if len(generatedRequest.GetSources()) != 1 || generatedRequest.GetSources()[0].GetGoogleDocs() == nil || generatedRequest.GetSources()[0].GetGoogleDocs().GetMimeType() != "application/vnd.google-apps.document" {
			t.Fatalf("%s:%d: Google Docs source variant missing", path, record)
		}
		encoded, err := json.Marshal(method.EncodeAddSourcesArgs(&generatedRequest))
		if err != nil {
			t.Fatal(err)
		}
		if string(encoded) != string(rawRequest) {
			t.Fatalf("%s:%d: generated Google Docs encoder does not match the live capture", path, record)
		}

		responseBody := entry.Response.Content.Text
		if entry.Response.Content.Encoding == "base64" {
			decoded, err := base64.StdEncoding.DecodeString(responseBody)
			if err != nil {
				t.Fatalf("%s:%d: base64 response: %v", path, record, err)
			}
			responseBody = string(decoded)
		}
		responseWire, err := batchexecute.DecodeResponse(responseBody)
		if err != nil {
			t.Fatalf("%s:%d: decode response: %v", path, record, err)
		}
		for _, response := range responseWire.Responses {
			if response.ID != "izAoDd" {
				continue
			}
			var generatedResponse pb.AddSourcesResponse
			if err := beprotojson.Unmarshal(response.Data, &generatedResponse); err != nil {
				t.Fatalf("%s:%d: generated response decode: %v", path, record, err)
			}
			if len(generatedResponse.GetSources()) != 1 || sourceID(generatedResponse.GetSources()[0]) == "" || sourceTitle(generatedResponse.GetSources()[0]) == "" {
				t.Fatalf("%s:%d: Google Docs source identity missing", path, record)
			}
			variants++
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if variants < 1 {
		t.Fatalf("Google Docs AddSources variants=%d, want at least 1", variants)
	}
}

func TestAddSourcesGoogleDocsScrubbedProjection(t *testing.T) {
	raw := loadFixture(t, "add_sources_google_docs_response_scrubbed.json")
	var response pb.AddSourcesResponse
	if err := beprotojson.Unmarshal(raw, &response); err != nil {
		t.Fatal(err)
	}
	if len(response.GetSources()) != 1 || sourceID(response.GetSources()[0]) != "scrubbed" || sourceTitle(response.GetSources()[0]) != "scrubbed" {
		t.Fatal("scrubbed Google Docs response projection differs")
	}
}

func TestLoadSourceGoogleDocsCorpusProjection(t *testing.T) {
	path := filepath.Join("/tmp", "nlm-traffic", "new-rec3", "notebooklm.google.com", "notebooklm.google.com.jsonl")
	docSourceIDs := googleDocsSourceIDs(t, path)
	if len(docSourceIDs) == 0 {
		t.Skip("no Google Docs AddSources result in the capture")
	}
	f, err := os.Open(path)
	if err != nil {
		t.Skipf("open live Google Docs capture: %v", err)
	}
	defer f.Close()

	variants := 0
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 64*1024*1024)
	for record := 1; scanner.Scan(); record++ {
		var entry struct {
			Request struct {
				URL string `json:"url"`
			} `json:"request"`
			Response struct {
				Content struct {
					Text     string `json:"text"`
					Encoding string `json:"encoding"`
				} `json:"content"`
			} `json:"response"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil || !strings.Contains(entry.Request.URL, "rpcids=hizoJc") {
			continue
		}
		body := entry.Response.Content.Text
		if entry.Response.Content.Encoding == "base64" {
			decoded, err := base64.StdEncoding.DecodeString(body)
			if err != nil {
				t.Fatalf("%s:%d: base64 response: %v", path, record, err)
			}
			body = string(decoded)
		}
		wire, err := batchexecute.DecodeResponse(body)
		if err != nil {
			continue
		}
		for _, response := range wire.Responses {
			if response.ID != "hizoJc" {
				continue
			}
			legacy, err := decodeLoadSourceText(response.Data)
			if err != nil || !docSourceIDs[legacy.SourceID] {
				continue
			}
			var generated pb.LoadSourceResponse
			if err := beprotojson.Unmarshal(response.Data, &generated); err != nil {
				t.Fatalf("%s:%d: generated decode: %v", path, record, err)
			}
			if legacy.SourceID == "" || legacy.Title == "" || len(legacy.Fragments) == 0 {
				t.Fatalf("%s:%d: Google Docs legacy projection incomplete", path, record)
			}
			got := loadSourceTextFromProto(&generated)
			if legacy.SourceID != got.SourceID || legacy.Title != got.Title || len(legacy.Fragments) != len(got.Fragments) {
				t.Fatalf("%s:%d: generated=%+v legacy=%+v", path, record, got, legacy)
			}
			for i := range legacy.Fragments {
				if legacy.Fragments[i] != got.Fragments[i] {
					t.Fatalf("%s:%d fragment %d: generated=%+v legacy=%+v", path, record, i, got.Fragments[i], legacy.Fragments[i])
				}
			}
			variants++
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if variants < 1 {
		t.Fatalf("Google Docs LoadSource variants=%d, want at least 1", variants)
	}
}

func googleDocsSourceIDs(t *testing.T, path string) map[string]bool {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Skipf("open live Google Docs capture: %v", err)
	}
	defer f.Close()

	ids := make(map[string]bool)
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 64*1024*1024)
	for record := 1; scanner.Scan(); record++ {
		var entry struct {
			Request struct {
				URL      string `json:"url"`
				PostData struct {
					Text string `json:"text"`
				} `json:"postData"`
			} `json:"request"`
			Response struct {
				Content struct {
					Text     string `json:"text"`
					Encoding string `json:"encoding"`
				} `json:"content"`
			} `json:"response"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil || !strings.Contains(entry.Request.URL, "rpcids=izAoDd") {
			continue
		}
		body, err := base64.StdEncoding.DecodeString(entry.Request.PostData.Text)
		if err != nil {
			t.Fatalf("%s:%d: base64 request: %v", path, record, err)
		}
		request, err := batchexecute.DecodeRequest(string(body))
		if err != nil {
			t.Fatalf("%s:%d: decode request: %v", path, record, err)
		}
		isGoogleDoc := false
		for _, call := range request.RPCs {
			if call.ID == "izAoDd" && strings.Contains(string(call.Args), `"application/vnd.google-apps.document"`) {
				isGoogleDoc = true
			}
		}
		if !isGoogleDoc {
			continue
		}
		responseBody := entry.Response.Content.Text
		if entry.Response.Content.Encoding == "base64" {
			decoded, err := base64.StdEncoding.DecodeString(responseBody)
			if err != nil {
				t.Fatalf("%s:%d: base64 response: %v", path, record, err)
			}
			responseBody = string(decoded)
		}
		wire, err := batchexecute.DecodeResponse(responseBody)
		if err != nil {
			continue
		}
		for _, response := range wire.Responses {
			if response.ID != "izAoDd" {
				continue
			}
			var generated pb.AddSourcesResponse
			if err := beprotojson.Unmarshal(response.Data, &generated); err != nil {
				t.Fatalf("%s:%d: decode AddSources response: %v", path, record, err)
			}
			for _, source := range generated.GetSources() {
				if id := sourceID(source); id != "" {
					ids[id] = true
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return ids
}
