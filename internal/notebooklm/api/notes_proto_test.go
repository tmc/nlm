package api

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/batchexecute"
	"github.com/tmc/nlm/internal/beprotojson"
)

func TestGetNotesProtoAdapterMatchesLegacyParser(t *testing.T) {
	raw := []byte(`[[["note-1",["note-1","body",[1,"157962509464",[1775436871,282578000],null,null,[1775436871,282578000],false],null,"Title","Rich",[1]]],["note-2",["note-2","",null,null,"Second","",[2]]]]]`)

	legacy, err := parseNotesResponse(raw)
	if err != nil {
		t.Fatalf("legacy parser: %v", err)
	}
	var wire pb.GetNotesWireResponse
	if err := beprotojson.Unmarshal(raw, &wire); err != nil {
		t.Fatalf("proto decoder: %v", err)
	}
	got := notesFromWireResponse(&wire)
	assertEquivalent(t, "notes adaptation", legacy, got)
}

func TestNotesFromWireResponseNilAndEmpty(t *testing.T) {
	if got := notesFromWireResponse(nil); got != nil {
		t.Fatalf("nil response = %#v, want nil", got)
	}
	got := notesFromWireResponse(&pb.GetNotesWireResponse{Entries: []*pb.GetNotesEntry{nil}})
	if got == nil || len(got) != 0 {
		t.Fatalf("empty entries = %#v, want non-nil empty slice", got)
	}
}

func TestGetNotesProtoAdapterCorpusEquivalence(t *testing.T) {
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
			if json.Unmarshal(scanner.Bytes(), &entry) != nil || !strings.Contains(entry.Request.URL, "rpcids=cFji9") {
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
			wireResponse, err := batchexecute.DecodeResponse(body)
			if err != nil {
				continue
			} // transport-empty capture
			for _, rpcResponse := range wireResponse.Responses {
				if rpcResponse.ID != "cFji9" || len(rpcResponse.Data) == 0 {
					continue
				}
				responses++
				legacy, err := parseNotesResponse(rpcResponse.Data)
				if err != nil {
					t.Fatalf("%s:%d: legacy parse: %v", file, record, err)
				}
				var wire pb.GetNotesWireResponse
				if err := beprotojson.Unmarshal(rpcResponse.Data, &wire); err != nil {
					t.Fatalf("%s:%d: proto decode: %v", file, record, err)
				}
				assertEquivalent(t, fmt.Sprintf("%s:%d", file, record), legacy, notesFromWireResponse(&wire))
			}
		}
		if err := scanner.Err(); err != nil {
			t.Fatal(err)
		}
		f.Close()
	}
	if responses != 6 {
		t.Fatalf("cFji9 responses=%d, want 6 non-empty captures", responses)
	}
}
