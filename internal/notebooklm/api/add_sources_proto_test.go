package api

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/batchexecute"
	"github.com/tmc/nlm/internal/beprotojson"
)

func TestAddSourcesProtoAdapterCorpusProjection(t *testing.T) {
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
	responses := 0
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
					URL string `json:"url"`
				} `json:"request"`
				Response struct {
					Content struct{ Text, Encoding string } `json:"content"`
				} `json:"response"`
			}
			if json.Unmarshal(scanner.Bytes(), &entry) != nil || !strings.Contains(entry.Request.URL, "rpcids=izAoDd") {
				continue
			}
			body := entry.Response.Content.Text
			if entry.Response.Content.Encoding == "base64" {
				decoded, err := base64.StdEncoding.DecodeString(body)
				if err != nil {
					t.Fatalf("%s:%d: base64 response: %v", file, record, err)
				}
				body = string(decoded)
			}
			wire, err := batchexecute.DecodeResponse(body)
			if err != nil {
				continue
			}
			for _, response := range wire.Responses {
				if response.ID != "izAoDd" || len(response.Data) == 0 {
					continue
				}
				responses++
				var raw []interface{}
				if err := json.Unmarshal(response.Data, &raw); err != nil {
					t.Fatalf("%s:%d: raw response: %v", file, record, err)
				}
				legacy := addSourcesIDsAndTitles(raw)
				var generated pb.AddSourcesResponse
				if err := beprotojson.Unmarshal(response.Data, &generated); err != nil {
					t.Fatalf("%s:%d: proto decode: %v", file, record, err)
				}
				if len(generated.GetSources()) != len(legacy) {
					t.Fatalf("%s:%d source count generated=%d legacy=%d", file, record, len(generated.GetSources()), len(legacy))
				}
				for i, source := range generated.GetSources() {
					if source == nil || source.GetSourceId() == nil || source.GetSourceId().GetSourceId() != legacy[i].ID || source.GetTitle() != legacy[i].Title {
						t.Fatalf("%s:%d source %d generated=(%q,%q) legacy=(%q,%q)", file, record, i, sourceID(source), sourceTitle(source), legacy[i].ID, legacy[i].Title)
					}
				}
			}
		}
		if err := scanner.Err(); err != nil {
			t.Fatal(err)
		}
		f.Close()
	}
	if responses < 1 {
		t.Fatalf("izAoDd responses=%d, want at least 1", responses)
	}
}

type addSourceIdentity struct{ ID, Title string }

func addSourcesIDsAndTitles(raw []interface{}) []addSourceIdentity {
	if len(raw) == 0 {
		return nil
	}
	if encoded, ok := raw[0].(string); ok {
		var nested []interface{}
		if json.Unmarshal([]byte(encoded), &nested) == nil {
			return addSourcesIDsAndTitles(nested)
		}
	}
	return collectAddSourceIdentities(raw)
}

func collectAddSourceIdentities(value interface{}) []addSourceIdentity {
	fields, ok := value.([]interface{})
	if !ok {
		return nil
	}
	if len(fields) >= 2 {
		ids, idsOK := fields[0].([]interface{})
		title, titleOK := fields[1].(string)
		if idsOK && titleOK {
			id, _ := stringValue(ids, 0)
			return []addSourceIdentity{{ID: id, Title: title}}
		}
	}
	var out []addSourceIdentity
	for _, child := range fields {
		out = append(out, collectAddSourceIdentities(child)...)
	}
	return out
}

func stringValue(values []interface{}, index int) (string, bool) {
	if index < 0 || index >= len(values) {
		return "", false
	}
	s, ok := values[index].(string)
	return s, ok
}

func sourceID(source *pb.Source) string {
	if source == nil || source.GetSourceId() == nil {
		return ""
	}
	return source.GetSourceId().GetSourceId()
}

func sourceTitle(source *pb.Source) string {
	if source == nil {
		return ""
	}
	return source.GetTitle()
}
