package main

import "testing"

// TestCitationContentKeyMatchesAcrossWhitespace pins the property chat-show's
// citation rehydration relies on: the locally-persisted answer text and the
// same answer refetched from GetConversationHistory can differ in whitespace
// (the history frame strips/rewrites newlines), so the match key must collapse
// whitespace and still agree. It must also distinguish different turns.
func TestCitationContentKeyMatchesAcrossWhitespace(t *testing.T) {
	persisted := "### Candidate Skills Triage List: Pass 3\n\nA rigorous third pass over the logs."
	history := "### Candidate Skills Triage List: Pass 3   A rigorous third pass over the logs."
	if citationContentKey(persisted) != citationContentKey(history) {
		t.Errorf("keys differ across whitespace:\n persisted=%q\n history  =%q",
			citationContentKey(persisted), citationContentKey(history))
	}

	other := "### Candidate Skills Triage List: Pass 2\n\nA different earlier pass."
	if citationContentKey(persisted) == citationContentKey(other) {
		t.Error("distinct turns collapsed to the same key")
	}

	// Empty content yields a stable (empty) key; the map lookup for a message
	// with no content simply misses and falls back to persisted citations.
	if citationContentKey("") != "" {
		t.Errorf("empty content key = %q, want empty", citationContentKey(""))
	}
}
