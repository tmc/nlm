package api

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tmc/nlm/internal/batchexecute"
)

func TestParseAppArtifactKind(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   string
		want AppArtifactKind
	}{
		{"prototype", AppArtifactKindPrototype},
		{"notebook-app", AppArtifactKindPrototype},
		{"mindmap", AppArtifactKindMindmap},
		{"mind-map", AppArtifactKindMindmap},
		{"canvas", AppArtifactKindCanvas},
	}
	for _, tt := range tests {
		got, err := ParseAppArtifactKind(tt.in)
		if err != nil {
			t.Fatalf("ParseAppArtifactKind(%q): %v", tt.in, err)
		}
		if got != tt.want {
			t.Fatalf("ParseAppArtifactKind(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestParseAppArtifactKindRejectsUnknown(t *testing.T) {
	t.Parallel()

	if _, err := ParseAppArtifactKind("flashcards"); err == nil {
		t.Fatal("ParseAppArtifactKind(flashcards) succeeded, want error")
	}
}

func TestCreatedArtifactIDFromProto(t *testing.T) {
	t.Parallel()

	raw := []byte(`[[ "artifact-1", "Title", 5 ]]`)
	got, err := createdArtifactIDFromProto(raw)
	if err != nil {
		t.Fatalf("createdArtifactIDFromProto: %v", err)
	}
	if got != "artifact-1" {
		t.Fatalf("artifact id = %q, want artifact-1", got)
	}
}

// TestCreatedArtifactIDFromProtoRejectsEmpty verifies that a blank id — what the
// server returns when a create is rejected without an RPC-level error (e.g.
// quota exhausted) — surfaces as an error instead of a silent empty id.
func TestCreatedArtifactIDFromProtoRejectsEmpty(t *testing.T) {
	t.Parallel()

	for _, resp := range []string{`[""]`, `[["", "Title", 5]]`, `[]`, `[[]]`} {
		if got, err := createdArtifactIDFromProto([]byte(resp)); err == nil || got != "" {
			t.Errorf("createdArtifactIDFromProto(%s) = %q, %v; want empty id and error", resp, got, err)
		}
	}
}

func TestCreatedArtifactIDProtoCorpus(t *testing.T) {
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

	successful := 0
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
			if json.Unmarshal(scanner.Bytes(), &entry) != nil || !strings.Contains(entry.Request.URL, "rpcids=R7cb6c") {
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
				if response.ID != "R7cb6c" || len(response.Data) == 0 {
					continue
				}
				id, err := createdArtifactIDFromProto(response.Data)
				if err != nil {
					continue // quota or validation rejection: covered by the unit cases
				}
				if id == "" {
					t.Fatalf("%s:%d: generated empty artifact ID", file, record)
				}
				successful++
			}
		}
		if err := scanner.Err(); err != nil {
			t.Fatal(err)
		}
		f.Close()
	}
	if successful < 6 {
		t.Fatalf("R7cb6c successful responses=%d, want at least 6", successful)
	}
}
