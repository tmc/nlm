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
)

func TestStartDeepResearchEncoderCorpusShape(t *testing.T) {
	path := filepath.Join("/tmp", "nlm-traffic", "new-rec3", "notebooklm.google.com", "notebooklm.google.com.jsonl")
	f, err := os.Open(path)
	if err != nil {
		t.Skipf("open live QA9ei capture: %v", err)
	}
	defer f.Close()

	captures := 0
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
		}
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil || !strings.Contains(entry.Request.URL, "rpcids=QA9ei") {
			continue
		}
		body, err := base64.StdEncoding.DecodeString(entry.Request.PostData.Text)
		if err != nil {
			t.Fatalf("%s:%d: base64 request: %v", path, record, err)
		}
		wire, err := batchexecute.DecodeRequest(string(body))
		if err != nil {
			t.Fatalf("%s:%d: decode request: %v", path, record, err)
		}
		for _, call := range wire.RPCs {
			if call.ID != "QA9ei" {
				continue
			}
			var args []interface{}
			if err := json.Unmarshal(call.Args, &args); err != nil {
				t.Fatalf("%s:%d: decode QA9ei args: %v", path, record, err)
			}
			if len(args) != 5 {
				t.Fatalf("%s:%d: QA9ei args=%d, want 5", path, record, len(args))
			}
			query, ok := args[2].([]interface{})
			if !ok || len(query) != 2 {
				t.Fatalf("%s:%d: QA9ei query shape", path, record)
			}
			queryText, ok := query[0].(string)
			if !ok {
				t.Fatalf("%s:%d: QA9ei query text", path, record)
			}
			projectID, ok := args[4].(string)
			if !ok {
				t.Fatalf("%s:%d: QA9ei project ID", path, record)
			}
			got := method.EncodeStartDeepResearchWireArgs(&pb.StartDeepResearchWireRequest{
				Context:   conversationRequestContext(),
				Query:     &pb.ResearchQuery{Query: queryText, Mode: 1},
				Mode:      5,
				ProjectId: projectID,
			})
			encoded, err := json.Marshal(got)
			if err != nil {
				t.Fatalf("%s:%d: encode generated QA9ei args: %v", path, record, err)
			}
			if string(encoded) != string(call.Args) {
				t.Fatalf("%s:%d: generated QA9ei encoder does not match the live capture", path, record)
			}
			captures++
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if captures < 1 {
		t.Fatalf("QA9ei captures=%d, want at least 1", captures)
	}
}
