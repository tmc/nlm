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

func TestGetNotesProtoAdapterProjection(t *testing.T) {
	raw := []byte(`[[["note-1",["note-1","body",[1,"157962509464",[1775436871,282578000],null,null,[1775436871,282578000],false],null,"Title","Rich",[1]]],["note-2",["note-2","",null,null,"Second","",[2]]]]]`)
	var wire pb.GetNotesRichWireResponse
	if err := beprotojson.Unmarshal(raw, &wire); err != nil {
		t.Fatalf("proto decoder: %v", err)
	}
	got := notesFromWireResponse(&wire)
	want := []*pb.Note{
		{NoteId: "note-1", ContentText: "body", Title: "Title", RichText: "Rich"},
		{NoteId: "note-2", Title: "Second"},
	}
	assertEquivalent(t, "notes adaptation", want, got)
}

func TestNotesFromWireResponseNilAndEmpty(t *testing.T) {
	if got := notesFromWireResponse(nil); got != nil {
		t.Fatalf("nil response = %#v, want nil", got)
	}
	got := notesFromWireResponse(&pb.GetNotesRichWireResponse{Entries: []*pb.GetNotesRichEntry{nil}})
	if got == nil || len(got) != 0 {
		t.Fatalf("empty entries = %#v, want non-nil empty slice", got)
	}
}

func TestGetNotesProtoAdapterCorpusProjection(t *testing.T) {
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
				var wire pb.GetNotesRichWireResponse
				if err := beprotojson.Unmarshal(rpcResponse.Data, &wire); err != nil {
					t.Fatalf("%s:%d: proto decode: %v", file, record, err)
				}
				got := notesFromWireResponse(&wire)
				if len(got) > len(wire.GetEntries()) {
					t.Fatalf("%s:%d: notes=%d entries=%d", file, record, len(got), len(wire.GetEntries()))
				}
				for i, note := range got {
					if note.GetNoteId() == "" {
						t.Fatalf("%s:%d: note %d has no ID", file, record, i)
					}
				}
			}
		}
		if err := scanner.Err(); err != nil {
			t.Fatal(err)
		}
		f.Close()
	}
	if responses < 6 {
		t.Fatalf("cFji9 responses=%d, want at least 6 non-empty captures", responses)
	}
}
