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

	"github.com/tmc/nlm/gen/method"
	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/batchexecute"
	"github.com/tmc/nlm/internal/beprotojson"
	"google.golang.org/protobuf/proto"
)

func TestGetConversationHistoryRequestEncoder(t *testing.T) {
	conversationID := "00000000-0000-4000-8000-000000000501"
	got := method.EncodeGetConversationHistoryArgs(&pb.GetConversationHistoryRequest{
		Context:        conversationRequestContext(),
		ConversationId: conversationID,
		Limit:          proto.Int32(20),
	})
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf(`[[2,null,[1],[1,null,null,null,null,null,null,null,null,null,[1,3]]],null,null,%q,20]`, conversationID)
	if string(encoded) != want {
		t.Fatalf("conversation history args = %s, want %s", encoded, want)
	}
}

func TestConversationHistoryProtoAdapterCorpusEquivalence(t *testing.T) {
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
			if json.Unmarshal(scanner.Bytes(), &entry) != nil || !strings.Contains(entry.Request.URL, "rpcids=khqZz") {
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
				if response.ID != "khqZz" || len(response.Data) == 0 {
					continue
				}
				responses++
				legacy, err := parseConversationHistoryLegacy(response.Data)
				if err != nil {
					t.Fatalf("%s:%d: legacy parse: %v", file, record, err)
				}
				var generated pb.GetConversationHistoryResponse
				if err := beprotojson.Unmarshal(response.Data, &generated); err != nil {
					t.Fatalf("%s:%d: proto decode: %v", file, record, err)
				}
				assertEquivalent(t, fmt.Sprintf("%s:%d", file, record), legacy, conversationMessagesFromProto(&generated))
			}
		}
		if err := scanner.Err(); err != nil {
			t.Fatal(err)
		}
		f.Close()
	}
	if responses != 4 {
		t.Fatalf("khqZz responses=%d, want 4", responses)
	}
}
