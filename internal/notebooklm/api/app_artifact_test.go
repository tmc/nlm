package api

import (
	"testing"
)

func TestParseAppArtifactKind(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   string
		want AppArtifactKind
	}{
		{"prototype", AppArtifactKindPrototype},
		{"notebook-app", AppArtifactKindPrototype},
		{"mindmap", AppArtifactKindMindmap},
		{"mind-map", AppArtifactKindMindmap},
		{"canvas", AppArtifactKindCanvas},
	}
	for _, tt := range tests {
		got, err := ParseAppArtifactKind(tt.in)
		if err != nil {
			t.Fatalf("ParseAppArtifactKind(%q): %v", tt.in, err)
		}
		if got != tt.want {
			t.Fatalf("ParseAppArtifactKind(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestParseAppArtifactKindRejectsUnknown(t *testing.T) {
	t.Parallel()

	if _, err := ParseAppArtifactKind("flashcards"); err == nil {
		t.Fatal("ParseAppArtifactKind(flashcards) succeeded, want error")
	}
}

func TestCreatedArtifactIDFromProto(t *testing.T) {
	t.Parallel()

	raw := []byte(`[[ "artifact-1", "Title", 5 ]]`)
	got, err := createdArtifactIDFromProto(raw)
	if err != nil {
		t.Fatalf("createdArtifactIDFromProto: %v", err)
	}
	if got != "artifact-1" {
		t.Fatalf("artifact id = %q, want artifact-1", got)
	}
}

// TestCreatedArtifactIDFromProtoRejectsEmpty verifies that a blank id — what the
// server returns when a create is rejected without an RPC-level error (e.g.
// quota exhausted) — surfaces as an error instead of a silent empty id.
func TestCreatedArtifactIDFromProtoRejectsEmpty(t *testing.T) {
	t.Parallel()

	for _, resp := range []string{`[""]`, `[["", "Title", 5]]`, `[]`, `[[]]`} {
		if got, err := createdArtifactIDFromProto([]byte(resp)); err == nil || got != "" {
			t.Errorf("createdArtifactIDFromProto(%s) = %q, %v; want empty id and error", resp, got, err)
		}
	}
}
