package api

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tmc/nlm/gen/method"
	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/batchexecute"
	"github.com/tmc/nlm/internal/beprotojson"
)

func TestLoadSourceRequestEncoder(t *testing.T) {
	req := &pb.LoadSourceRequest{
		Source:  &pb.SourceIdList{SourceId: "source-1"},
		Mode:    &pb.Int32List{Value: 2},
		Context: conversationRequestContext(),
	}
	got, err := json.Marshal(method.EncodeLoadSourceArgs(req))
	if err != nil {
		t.Fatal(err)
	}
	want := `[["source-1"],[2],[2,null,[1],[1,null,null,null,null,null,null,null,null,null,[1,3]]]]`
	if string(got) != want {
		t.Fatalf("load source args = %s, want %s", got, want)
	}
}

func TestLoadSourceTextProtoAdapterCorpusEquivalence(t *testing.T) {
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
	fallbacks := 0
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
			if json.Unmarshal(scanner.Bytes(), &entry) != nil || !strings.Contains(entry.Request.URL, "rpcids=hizoJc") {
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
				if response.ID != "hizoJc" || len(response.Data) == 0 {
					continue
				}
				responses++
				legacy, err := decodeLoadSourceText(response.Data)
				if err != nil {
					t.Fatalf("%s:%d: legacy decode: %v", file, record, err)
				}
				var generated pb.LoadSourceResponse
				if err := beprotojson.Unmarshal(response.Data, &generated); err != nil {
					fallbacks++
					continue
				}
				got := loadSourceTextFromProto(&generated)
				if legacy.SourceID != got.SourceID || legacy.Title != got.Title || len(legacy.Fragments) != len(got.Fragments) {
					t.Fatalf("%s:%d: generated=%+v legacy=%+v", file, record, got, legacy)
				}
				for i := range legacy.Fragments {
					if legacy.Fragments[i] != got.Fragments[i] {
						t.Fatalf("%s:%d fragment %d: generated=%+v legacy=%+v", file, record, i, got.Fragments[i], legacy.Fragments[i])
					}
				}
			}
		}
		if err := scanner.Err(); err != nil {
			t.Fatal(err)
		}
		f.Close()
	}
	if responses < 2 {
		t.Fatalf("hizoJc responses=%d, want at least 2", responses)
	}
	if fallbacks == 0 {
		t.Log("all observed hizoJc responses used the generated decoder")
	}
}
