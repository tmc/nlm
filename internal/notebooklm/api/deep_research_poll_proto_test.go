package api

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	method "github.com/tmc/nlm/gen/method"
	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/batchexecute"
	"github.com/tmc/nlm/internal/beprotojson"
)

func TestPollDeepResearchProtoCorpusShadow(t *testing.T) {
	path := filepath.Join("/tmp", "nlm-traffic", "new-rec3", "notebooklm.google.com", "notebooklm.google.com.jsonl")
	jobHandle := deepResearchPollJobHandle(t, path)
	if jobHandle == "" {
		t.Skip("no deep-research poll handle in the capture")
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	polls := 0
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
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil || !strings.Contains(entry.Request.URL, "rpcids=e3bVqc") {
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
			if call.ID == "e3bVqc" {
				rawRequest = call.Args
				break
			}
		}
		if len(rawRequest) == 0 {
			continue
		}
		var pollRequest pb.PollDeepResearchRequest
		if err := beprotojson.Unmarshal(rawRequest, &pollRequest); err != nil || pollRequest.GetContext() == nil || pollRequest.GetJobHandle() != jobHandle {
			continue
		}
		encoded, err := json.Marshal(method.EncodePollDeepResearchArgs(&pollRequest))
		if err != nil {
			t.Fatal(err)
		}
		if string(encoded) != string(rawRequest) {
			t.Fatalf("%s:%d: generated poll encoder does not match the live capture", path, record)
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
			t.Fatalf("%s:%d: decode response: %v", path, record, err)
		}
		for _, response := range wire.Responses {
			if response.ID != "e3bVqc" || len(response.Data) == 0 {
				continue
			}
			legacy, err := parseDeepResearchSessions(response.Data, false)
			if err != nil {
				t.Fatalf("%s:%d: legacy decode: %v", path, record, err)
			}
			var generated pb.PollDeepResearchResponse
			if err := beprotojson.Unmarshal(response.Data, &generated); err != nil {
				t.Fatalf("%s:%d: generated decode: %v", path, record, err)
			}
			got := deepResearchSessionsFromProto(generated.GetSessions())
			var sessions pb.GetDeepResearchSessionsResponse
			if err := beprotojson.Unmarshal(response.Data, &sessions); err != nil {
				t.Fatalf("%s:%d: generated sessions decode: %v", path, record, err)
			}
			if !reflect.DeepEqual(got, deepResearchSessionsFromProto(sessions.GetSessions())) {
				t.Fatalf("%s:%d: poll and sessions proto projections differ", path, record)
			}
			assertDeepResearchSessionsEqual(t, path, record, got, legacy)
			polls++
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if polls < 19 {
		t.Fatalf("deep-research polls=%d, want at least 19", polls)
	}
}

func TestPollDeepResearchProtoScrubbedFixture(t *testing.T) {
	raw := loadFixture(t, "e3bVqc_poll_response_scrubbed.json")
	legacy, err := parseDeepResearchSessions(raw, false)
	if err != nil {
		t.Fatal(err)
	}
	var generated pb.PollDeepResearchResponse
	if err := beprotojson.Unmarshal(raw, &generated); err != nil {
		t.Fatal(err)
	}
	got := deepResearchSessionsFromProto(generated.GetSessions())
	assertDeepResearchSessionsEqual(t, "scrubbed fixture", 0, got, legacy)
	if len(got) == 0 || got[0].Report == "" || len(got[0].Sources) == 0 {
		t.Fatal("scrubbed fixture does not exercise the completed report projection")
	}
}

func deepResearchPollJobHandle(t *testing.T, path string) string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Skipf("open live deep-research capture: %v", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 64*1024*1024)
	for record := 1; scanner.Scan(); record++ {
		if record != 31 { // 0-based line 30: first poll after the QA9ei kickoff.
			continue
		}
		var entry struct {
			Request struct {
				URL      string `json:"url"`
				PostData struct {
					Text string `json:"text"`
				} `json:"postData"`
			} `json:"request"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(entry.Request.URL, "rpcids=e3bVqc") {
			t.Fatalf("%s:%d: expected e3bVqc poll", path, record)
		}
		body, err := base64.StdEncoding.DecodeString(entry.Request.PostData.Text)
		if err != nil {
			t.Fatalf("%s:%d: base64 poll request: %v", path, record, err)
		}
		wire, err := batchexecute.DecodeRequest(string(body))
		if err != nil {
			t.Fatalf("%s:%d: decode poll request: %v", path, record, err)
		}
		for _, call := range wire.RPCs {
			if call.ID != "e3bVqc" {
				continue
			}
			var poll pb.PollDeepResearchRequest
			if err := beprotojson.Unmarshal(call.Args, &poll); err != nil {
				t.Fatalf("%s:%d: decode poll request: %v", path, record, err)
			}
			return poll.GetJobHandle()
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return ""
}

func assertDeepResearchSessionsEqual(t *testing.T, path string, record int, got, want []deepResearchSession) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s:%d: session count generated=%d legacy=%d", path, record, len(got), len(want))
	}
	for i := range want {
		expected := want[i]
		if len(expected.MainBlob) != 0 {
			expected.Report, expected.Sources = decodeDeepResearchContent(expected.MainBlob)
			if expected.Mode != 5 {
				expected.Report, expected.Sources = decodeFastMainBlob(expected.MainBlob)
			}
		}
		if got[i].ConversationID != expected.ConversationID || got[i].ProjectID != expected.ProjectID || got[i].Query != expected.Query || got[i].Mode != expected.Mode || got[i].State != expected.State || got[i].ResearchID != expected.ResearchID {
			t.Fatalf("%s:%d: session %d generated identity differs", path, record, i)
		}
		if !bytes.Equal(got[i].Plan, expected.Plan) {
			t.Fatalf("%s:%d: session %d generated plan differs", path, record, i)
		}
		if got[i].Report != expected.Report {
			t.Fatalf("%s:%d: session %d generated report differs", path, record, i)
		}
		if !reflect.DeepEqual(got[i].Sources, expected.Sources) {
			t.Fatalf("%s:%d: session %d generated sources differ (%d != %d)", path, record, i, len(got[i].Sources), len(expected.Sources))
		}
	}
}
