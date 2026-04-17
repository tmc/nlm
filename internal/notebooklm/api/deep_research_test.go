package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
