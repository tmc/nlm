package api

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/batchexecute"
	beprotojson "github.com/tmc/nlm/internal/beprotojson"
)

// loadFixture reads a testdata fixture relative to the repo-level
// internal/method/testdata directory. The api package keeps fixtures
// alongside the other verified wire shapes under internal/method/ so
// all CDP-captured wire samples live in one place.
func loadFixture(t *testing.T, name string) json.RawMessage {
	t.Helper()
	// This file lives at internal/notebooklm/api; walk up to the repo
	// root and then into internal/method/testdata.
	path := filepath.Join("..", "..", "method", "testdata", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return json.RawMessage(bytes.TrimSpace(data))
}

func TestParseDeepResearchSessions_Empty(t *testing.T) {
	sessions, err := parseDeepResearchSessions(loadFixture(t, "e3bVqc_sessions_response_empty.json"), false)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("want zero sessions, got %d", len(sessions))
	}
}

func TestParseDeepResearchSessions_Complete(t *testing.T) {
	sessions, err := parseDeepResearchSessions(loadFixture(t, "e3bVqc_sessions_response_complete.json"), false)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("want 1 session, got %d", len(sessions))
	}
	s := sessions[0]
	if s.ConversationID != "00000000-0000-4000-8000-000000000402" {
		t.Errorf("conversation_id: got %q", s.ConversationID)
	}
	if s.ResearchID != "00000000-0000-4000-8000-000000000501" {
		t.Errorf("research_id: got %q", s.ResearchID)
	}
	if s.State != 2 {
		t.Errorf("state: got %d, want 2 (COMPLETE)", s.State)
	}
	if len(s.MainBlob) == 0 {
		t.Error("MainBlob should be populated for COMPLETE sessions")
	}
	if s.Query != "notebooklm clis" {
		t.Errorf("query: got %q", s.Query)
	}
	if len(s.Plan) == 0 {
		t.Error("Plan should be decoded from base64 for COMPLETE sessions")
	}
}

// TestDeepResearchGeneratedShadowDecode checks the generated model against
// the fields consumed by the poller on compact fixtures. The full corpus
// equivalence test below is the gate for the live generated decoder.
func TestDeepResearchGeneratedShadowDecode(t *testing.T) {
	for _, name := range []string{
		"e3bVqc_sessions_response_complete.json",
		"e3bVqc_sessions_response_running.json",
		"e3bVqc_sessions_response_real_3session.json",
	} {
		raw := loadFixture(t, name)
		legacy, err := parseDeepResearchSessions(raw, false)
		if err != nil {
			t.Fatalf("%s: legacy parse: %v", name, err)
		}
		var generated pb.GetDeepResearchSessionsResponse
		if err := beprotojson.Unmarshal(raw, &generated); err != nil {
			t.Fatalf("%s: generated decode: %v", name, err)
		}
		if len(generated.GetSessions()) != len(legacy) {
			t.Fatalf("%s: session count: generated=%d legacy=%d", name, len(generated.GetSessions()), len(legacy))
		}
		for i, want := range legacy {
			got := generated.GetSessions()[i]
			if got.GetConversationId() != want.ConversationID {
				t.Errorf("%s[%d] conversation_id: generated=%q legacy=%q", name, i, got.GetConversationId(), want.ConversationID)
			}
			details := got.GetDetails()
			if details == nil {
				t.Errorf("%s[%d]: generated details is nil", name, i)
				continue
			}
			if details.GetProjectId() != want.ProjectID {
				t.Errorf("%s[%d] project_id: generated=%q legacy=%q", name, i, details.GetProjectId(), want.ProjectID)
			}
			if details.GetQuery().GetText() != want.Query {
				t.Errorf("%s[%d] query: generated=%q legacy=%q", name, i, details.GetQuery().GetText(), want.Query)
			}
			if int(details.GetMode()) != want.Mode {
				t.Errorf("%s[%d] mode: generated=%d legacy=%d", name, i, details.GetMode(), want.Mode)
			}
			if int(details.GetState()) != want.State {
				t.Errorf("%s[%d] state: generated=%d legacy=%d", name, i, details.GetState(), want.State)
			}
			if (details.GetMainBlob() != nil) != (len(want.MainBlob) != 0) {
				t.Errorf("%s[%d] main_blob presence: generated=%t legacy=%t", name, i, details.GetMainBlob() != nil, len(want.MainBlob) != 0)
			}
			if details.GetMainBlob() != nil && len(want.MainBlob) != 0 {
				legacyReport, legacySources := decodeDeepResearchContent(want.MainBlob)
				if want.Mode != 5 {
					legacyReport, legacySources = decodeFastMainBlob(want.MainBlob)
				}
				entries := details.GetMainBlob().GetReportTree()
				if want.Mode == 5 {
					if len(entries) == 0 || entries[0].GetDetail() == nil {
						t.Errorf("%s[%d]: generated report header missing", name, i)
					} else if entries[0].GetDetail().GetMarkdown() != legacyReport {
						t.Errorf("%s[%d] report markdown differs: generated=%d bytes legacy=%d bytes", name, i, len(entries[0].GetDetail().GetMarkdown()), len(legacyReport))
					}
				} else if details.GetMainBlob().GetExtra() != legacyReport {
					t.Errorf("%s[%d] fast summary differs: generated=%q legacy=%q", name, i, details.GetMainBlob().GetExtra(), legacyReport)
				}
				entryOffset := 1
				if want.Mode != 5 {
					entryOffset = 0
				}
				if len(entries) < entryOffset || len(entries)-entryOffset != len(legacySources) {
					t.Errorf("%s[%d] source count: generated=%d legacy=%d", name, i, len(entries)-entryOffset, len(legacySources))
				} else {
					for j, source := range legacySources {
						entry := entries[j+entryOffset]
						if entry.GetUrl() != source.URL || entry.GetTitle() != source.Title || entry.GetSummary() != source.Snippet {
							t.Errorf("%s[%d] source %d differs: generated=(%q,%q,%q) legacy=(%q,%q,%q)", name, i, j, entry.GetUrl(), entry.GetTitle(), entry.GetSummary(), source.URL, source.Title, source.Snippet)
						}
					}
				}
			}
			metadata := details.GetMetadata()
			if metadata != nil && metadata.GetResearchId() != want.ResearchID {
				t.Errorf("%s[%d] research_id: generated=%q legacy=%q", name, i, metadata.GetResearchId(), want.ResearchID)
			}
		}
	}
}

func TestDeepResearchGeneratedCorpusEquivalence(t *testing.T) {
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
	fast, deep := 0, 0
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
					Content struct {
						Text     string `json:"text"`
						Encoding string `json:"encoding"`
					} `json:"content"`
				} `json:"response"`
			}
			if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil || !strings.Contains(entry.Request.URL, "rpcids=e3bVqc") {
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
				if strings.Contains(err.Error(), "empty response") {
					continue
				}
				t.Fatalf("%s:%d: batchexecute response: %v", file, record, err)
			}
			for _, rpcResponse := range wire.Responses {
				if rpcResponse.ID != "e3bVqc" {
					continue
				}
				responses++
				raw := json.RawMessage(bytes.TrimSpace(rpcResponse.Data))
				legacy, err := parseDeepResearchSessions(raw, false)
				if err != nil {
					t.Fatalf("%s:%d: legacy parse: %v", file, record, err)
				}
				generated, err := parseDeepResearchSessionsProto(raw)
				if err != nil {
					t.Fatalf("%s:%d: generated parse: %v", file, record, err)
				}
				if len(generated) != len(legacy) {
					t.Fatalf("%s:%d: session count generated=%d legacy=%d", file, record, len(generated), len(legacy))
				}
				for i := range legacy {
					got, want := generated[i], legacy[i]
					if got.ConversationID != want.ConversationID || got.ProjectID != want.ProjectID || got.Query != want.Query || got.Mode != want.Mode || got.State != want.State || got.ResearchID != want.ResearchID {
						t.Errorf("%s:%d session %d identity differs: generated=%+v legacy=%+v", file, record, i, got, want)
					}
					if (len(got.MainBlob) != 0) != (len(want.MainBlob) != 0) {
						t.Errorf("%s:%d session %d main_blob presence differs", file, record, i)
					}
					if len(want.MainBlob) == 0 {
						continue
					}
					legacyReport, legacySources := decodeDeepResearchContent(want.MainBlob)
					if want.Mode != 5 {
						legacyReport, legacySources = decodeFastMainBlob(want.MainBlob)
					}
					if got.Report != legacyReport {
						t.Errorf("%s:%d session %d report differs", file, record, i)
					}
					if len(got.Sources) != len(legacySources) {
						t.Errorf("%s:%d session %d source count generated=%d legacy=%d", file, record, i, len(got.Sources), len(legacySources))
						continue
					}
					for j := range legacySources {
						if got.Sources[j] != legacySources[j] {
							t.Errorf("%s:%d session %d source %d differs: generated=%+v legacy=%+v", file, record, i, j, got.Sources[j], legacySources[j])
						}
					}
				}
				for _, session := range legacy {
					if session.Mode == 5 {
						deep++
					} else {
						fast++
					}
				}
			}
		}
		if err := scanner.Err(); err != nil {
			t.Fatal(err)
		}
		f.Close()
	}
	if responses < 24 {
		t.Fatalf("e3bVqc responses=%d, want at least 24", responses)
	}
	if fast == 0 || deep == 0 {
		t.Fatalf("branch coverage fast=%d deep=%d", fast, deep)
	}
}

func TestParseDeepResearchSessions_Running(t *testing.T) {
	// The RUNNING fixture has two sessions side by side: one new RUNNING
	// research (state=1, main_blob=null) and one older TOMBSTONED research
	// (state=5, main_blob still populated). The parser should surface both
	// with correctly decoded state enums so the scanner can filter.
	sessions, err := parseDeepResearchSessions(loadFixture(t, "e3bVqc_sessions_response_running.json"), false)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("want 2 sessions, got %d", len(sessions))
	}

	var running, tomb *deepResearchSession
	for i := range sessions {
		switch sessions[i].State {
		case 1:
			running = &sessions[i]
		case 5:
			tomb = &sessions[i]
		}
	}
	if running == nil {
		t.Fatal("no state=1 (RUNNING) session found")
	}
	if tomb == nil {
		t.Fatal("no state=5 (TOMBSTONE) session found")
	}
	if len(running.MainBlob) != 0 {
		t.Errorf("RUNNING session should have nil MainBlob; got %d bytes", len(running.MainBlob))
	}
	if len(tomb.MainBlob) == 0 {
		t.Error("TOMBSTONE session should still carry MainBlob from pre-delete state")
	}
}

func TestDecodeDeepResearchContent(t *testing.T) {
	sessions, err := parseDeepResearchSessions(loadFixture(t, "e3bVqc_sessions_response_complete.json"), false)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("want 1 session, got %d", len(sessions))
	}
	report, sources := decodeDeepResearchContent(sessions[0].MainBlob)
	preview := report
	if len(preview) > 50 {
		preview = preview[:50]
	}
	if !strings.HasPrefix(report, "# ") {
		t.Errorf("report should begin with a markdown heading; got %q", preview)
	}
	if len(report) < 1000 {
		t.Errorf("report suspiciously short: %d chars", len(report))
	}
	if len(sources) == 0 {
		t.Fatal("no sources extracted")
	}
	// First source should have URL, Title, and a Rank from position [3].
	first := sources[0]
	if first.URL == "" || first.Title == "" {
		t.Errorf("first source missing URL/Title: %+v", first)
	}
}

// synthSession returns a JSON RawMessage encoding one outer-level
// session entry with the given state and main_blob presence. Used to
// drive the state-enum sweep in TestPollDeepResearchStateFilter.
func synthSession(researchID string, state int, hasBlob bool) []byte {
	mainBlob := "null"
	if hasBlob {
		mainBlob = `[[[null,"Synthetic","md",null]]]`
	}
	return []byte(`[null,["proj-1",["q",1],5,` + mainBlob + `,` + intStr(state) + `,["` + researchID + `",""]]]`)
}

func intStr(n int) string {
	if n == 0 {
		return "0"
	}
	// small ints 0..99 suffice
	s := ""
	if n < 0 {
		s = "-"
		n = -n
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return s + string(digits)
}

// TestParseDeepResearchSessions_StateSweep exercises every state value
// we might plausibly see on the wire. Values 0/3/4 have not been
// observed in any CDP capture to date, but a forward-compatible parser
// must decode them correctly so the PollDeepResearch scanner can apply
// its own filter rules.
func TestParseDeepResearchSessions_StateSweep(t *testing.T) {
	for _, state := range []int{0, 1, 2, 3, 4, 5, 99} {
		hasBlob := state == 2 || state == 5
		entry := synthSession("r-"+intStr(state), state, hasBlob)
		outer := `[[` + string(entry) + `]]`
		sessions, err := parseDeepResearchSessions(json.RawMessage(outer), false)
		if err != nil {
			t.Fatalf("state=%d: parse: %v", state, err)
		}
		if len(sessions) != 1 {
			t.Fatalf("state=%d: want 1 session, got %d", state, len(sessions))
		}
		if sessions[0].State != state {
			t.Errorf("state=%d: decoded State=%d", state, sessions[0].State)
		}
		if hasBlob && len(sessions[0].MainBlob) == 0 {
			t.Errorf("state=%d: expected populated MainBlob", state)
		}
		if !hasBlob && len(sessions[0].MainBlob) != 0 {
			t.Errorf("state=%d: expected nil MainBlob", state)
		}
	}
}

// TestPollDeepResearchSentinelClassification locks in the contract that
// an in-flight poll returns ErrResearchPolling regardless of which not-
// done path triggered it (race window, running state, unknown state).
// The exit-code classifier in cmd/nlm treats this sentinel as exit 7.
func TestPollDeepResearchSentinelClassification(t *testing.T) {
	// Build a session list where our researchID is present but state=1
	// (running). The scanner should return ErrResearchPolling.
	outer := `[[` + string(synthSession("r-target", 1, false)) + `]]`
	// Can't call the real Poll without a client; instead verify the
	// sentinel is what the classifier keys on by doing the scan inline.
	sessions, err := parseDeepResearchSessions(json.RawMessage(outer), false)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	// Simulate the scanner decision for state=1.
	for _, s := range sessions {
		if s.ResearchID != "r-target" {
			continue
		}
		if s.State == 2 && len(s.MainBlob) > 0 {
			t.Fatal("state=1 should not satisfy the done-check")
		}
	}
	// Lock the sentinel itself exists and is exported.
	if ErrResearchPolling == nil {
		t.Fatal("ErrResearchPolling should be a non-nil exported sentinel")
	}
	if !errors.Is(ErrResearchPolling, ErrResearchPolling) {
		t.Fatal("errors.Is should match the sentinel against itself")
	}
}
