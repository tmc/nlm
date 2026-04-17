package api

import (
	"encoding/json"
	"testing"
)

// TestParseFastSessions_Running locks the e3bVqc fast-mode session shape
// while the research is still running (state=1, main_blob=null).
func TestParseFastSessions_Running(t *testing.T) {
	sessions, err := parseDeepResearchSessions(loadFixture(t, "e3bVqc_fast_sessions_running.json"), false)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var fast *deepResearchSession
	for i := range sessions {
		if sessions[i].ConversationID == "00000000-0000-4000-8000-000000000401" {
			fast = &sessions[i]
		}
	}
	if fast == nil {
		t.Fatal("no fast-mode session matched")
	}
	if fast.Mode != 1 {
		t.Errorf("Mode: got %d, want 1 (fast)", fast.Mode)
	}
	if fast.State != 1 {
		t.Errorf("State: got %d, want 1 (running)", fast.State)
	}
	if len(fast.MainBlob) != 0 {
		t.Errorf("MainBlob: want empty while running, got %d bytes", len(fast.MainBlob))
	}
	if fast.Query != "har harl file formats" {
		t.Errorf("Query: got %q", fast.Query)
	}
}

// TestParseFastSessions_Complete locks the fast-mode complete session
// and drives decodeFastMainBlob to decode the sources array.
func TestParseFastSessions_Complete(t *testing.T) {
	sessions, err := parseDeepResearchSessions(loadFixture(t, "e3bVqc_fast_sessions_complete.json"), false)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("sessions: got %d, want 1", len(sessions))
	}
	s := sessions[0]
	if s.Mode != 1 {
		t.Errorf("Mode: got %d, want 1", s.Mode)
	}
	if s.State != 2 {
		t.Errorf("State: got %d, want 2 (complete)", s.State)
	}
	if len(s.MainBlob) == 0 {
		t.Fatal("MainBlob empty; want sources array")
	}
	summary, sources := decodeFastMainBlob(s.MainBlob)
	if summary == "" {
		t.Error("summary: want non-empty trailer string")
	}
	if len(sources) < 3 {
		t.Fatalf("sources: got %d, want >=3", len(sources))
	}
	if sources[0].URL == "" || sources[0].Title == "" {
		t.Errorf("first source missing URL/Title: %+v", sources[0])
	}
	if sources[0].Rank == 0 {
		t.Errorf("first source Rank should be set; got %+v", sources[0])
	}
}

// TestStartFastResearchEncoderShape verifies the argument layout the
// StartFastResearch method emits against the HAR-captured request.
func TestStartFastResearchEncoderShape(t *testing.T) {
	want := canonicalJSON(t, loadFixture(t, "Ljjv0c_fast_research_request.json"))
	got := encodeStartFastResearchArgs(t, "har harl file formats", "00000000-0000-4000-8000-000000000006")
	if got != want {
		t.Errorf("encoder shape mismatch\n got: %s\nwant: %s", got, want)
	}
}

func encodeStartFastResearchArgs(t *testing.T, query, projectID string) string {
	t.Helper()
	args := startFastResearchArgs(query, projectID)
	buf, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(buf)
}
