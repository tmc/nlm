package main

import (
	"encoding/json"
	"testing"

	"github.com/tmc/nlm/notebooklm"
)

// TestPersistableCitationsBakesParentTitle checks that save-time title
// resolution uses the parent source while preserving both wire IDs.
func TestPersistableCitationsBakesParentTitle(t *testing.T) {
	const (
		chunk  = "cccccccc-1111-2222-3333-444444444444"
		parent = "11111111-2222-3333-4444-555555555555"
	)
	resolveTitle := func(id string) string {
		if id == parent {
			return "product-docs.md"
		}
		return ""
	}
	citations := []notebooklm.Citation{{SourceIndex: 1, SourceID: chunk, ParentSourceID: parent}}

	got := persistableCitations(citations, resolveTitle)
	if len(got) != 1 {
		t.Fatalf("got %d citations, want 1", len(got))
	}
	if got[0].Title != "product-docs.md" {
		t.Errorf("baked Title = %q, want product-docs.md", got[0].Title)
	}
	if got[0].SourceID != chunk || got[0].ParentSourceID != parent {
		t.Errorf("ids mutated: SourceID=%q ParentSourceID=%q", got[0].SourceID, got[0].ParentSourceID)
	}
}

func TestChatMessageCitationParentRoundTrip(t *testing.T) {
	const (
		chunk  = "cccccccc-1111-2222-3333-444444444444"
		parent = "11111111-2222-3333-4444-555555555555"
	)
	message := storedMessage{
		Role:    "assistant",
		Content: "Grounded claim. [1]",
		Citations: []notebooklm.Citation{
			{SourceIndex: 1, SourceID: chunk, ParentSourceID: parent, Title: "product-docs.md", Confidence: 0.9},
		},
	}

	data, err := json.Marshal(message)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got storedMessage
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Citations) != 1 {
		t.Fatalf("got %d citations, want 1", len(got.Citations))
	}
	citation := got.Citations[0]
	if citation.ParentSourceID != parent {
		t.Errorf("ParentSourceID = %q, want %q", citation.ParentSourceID, parent)
	}
	if citation.SourceID != chunk {
		t.Errorf("SourceID = %q, want %q", citation.SourceID, chunk)
	}
	if citation.Title != "product-docs.md" {
		t.Errorf("Title = %q, want product-docs.md", citation.Title)
	}
}
