package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/beprotojson"
)

func TestFlashcardDeckFromCapturedArtifactPath(t *testing.T) {
	t.Parallel()

	// Reduced from the 2026-07-27 v9rmvd capture. The app JSON is Artifact
	// field 10, inner field 4: zero-based wire path [0][9][3].
	raw := []byte(`[
		["artifact-1", "Photonics Flashcards", 4, [], 3, null, null, null, null,
			["<!doctype html>", [1, null, null, "en", null, null, [2, 2], null, true], null,
				"{\"flashcards\":[{\"f\":\"What is FDTDX?\",\"b\":\"An inverse-design FDTD package.\"},{\"f\":\"What framework does it use?\",\"b\":\"JAX.\"}],\"topics\":{\"covered\":[\"FDTD\"],\"followUp\":[\"Optimization\"]}}"
			]
		]
	]`)
	var artifact pb.Artifact
	if err := (beprotojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(raw, &artifact); err != nil {
		t.Fatalf("decode captured artifact: %v", err)
	}

	deck, err := flashcardDeckFromArtifact(&artifact)
	if err != nil {
		t.Fatalf("flashcardDeckFromArtifact() error = %v", err)
	}
	if deck.Title != "Photonics Flashcards" {
		t.Fatalf("title = %q", deck.Title)
	}
	if len(deck.Data.Flashcards) != 2 {
		t.Fatalf("cards = %d, want 2", len(deck.Data.Flashcards))
	}
	if got := deck.Data.Flashcards[0].Front; got != "What is FDTDX?" {
		t.Fatalf("first front = %q", got)
	}
	if got := deck.HTML; got != "<!doctype html>" {
		t.Fatalf("HTML = %q", got)
	}
}

func TestFlashcardDeckRejectsWrongShape(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		artifact *pb.Artifact
		want     string
	}{
		{"nil", nil, "empty"},
		{"wrong type", &pb.Artifact{ArtifactId: "a", Type: pb.ArtifactType_ARTIFACT_TYPE_9}, "not a type-4"},
		{"no data", &pb.Artifact{ArtifactId: "a", Type: pb.ArtifactType_ARTIFACT_TYPE_REPORT}, "no flashcard app data"},
		{"invalid JSON", appArtifact(`{`), "decode flashcard app data"},
		{"no cards", appArtifact(`{"flashcards":[]}`), "no flashcards"},
		{"empty front", appArtifact(`{"flashcards":[{"f":"","b":"back"}]}`), "empty front"},
		{"empty back", appArtifact(`{"flashcards":[{"f":"front","b":""}]}`), "empty back"},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := flashcardDeckFromArtifact(tt.artifact)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want substring %q", err, tt.want)
			}
		})
	}
}

func TestWriteFlashcardDeck(t *testing.T) {
	t.Parallel()

	deck := &flashcardDeck{
		ArtifactID: "artifact-1",
		Title:      "Test Deck",
		HTML:       "<!doctype html><title>Test Deck</title>",
		Data: flashcardData{
			Flashcards: []flashcard{
				{Front: "Front 1", Back: "Back 1"},
				{Front: "Front 2", Back: "Back 2"},
			},
			Topics: flashcardTopics{Covered: []string{"Testing"}},
		},
	}
	tests := []struct {
		format string
		want   string
	}{
		{"md", "# Test Deck\n\n## 1. Front 1\n\nBack 1\n\n"},
		{"json", `"flashcards"`},
		{"tsv", "Front 1\tBack 1\n"},
		{"html", "<!doctype html><title>Test Deck</title>"},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.format, func(t *testing.T) {
			t.Parallel()
			var out bytes.Buffer
			if err := writeFlashcardDeck(&out, deck, tt.format); err != nil {
				t.Fatalf("writeFlashcardDeck() error = %v", err)
			}
			if !strings.Contains(out.String(), tt.want) {
				t.Fatalf("output = %q, want substring %q", out.String(), tt.want)
			}
		})
	}
}

func TestParseArtifactExportArgs(t *testing.T) {
	t.Parallel()

	command, ok := lookupCommand("artifact export")
	if !ok {
		t.Fatal("artifact export command not found")
	}
	parsed, err := parseCommandSpec(command.spec, command.surfaceSpec, []string{
		"--format", "json",
		"artifact-1",
		"--output", "cards.json",
	}, globalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	args, err := decodeArtifactExportArgs(parsed)
	if err != nil {
		t.Fatal(err)
	}
	if args.ArtifactID != "artifact-1" || args.Options.Format != "json" || args.Options.Output != "cards.json" {
		t.Fatalf("arguments = %#v", args)
	}
}

func TestArtifactExportWriterDownloadsReadyArtifact(t *testing.T) {
	t.Parallel()

	reader := &fakeArtifactFileReader{content: "rendered document"}
	artifact := &pb.Artifact{
		ArtifactId: "artifact-1",
		Type:       pb.ArtifactType_ARTIFACT_TYPE_10,
		State:      pb.ArtifactState_ARTIFACT_STATE_READY,
	}
	write, err := artifactExportWriter(reader, artifact, "md")
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := write(&out); err != nil {
		t.Fatal(err)
	}
	if out.String() != reader.content {
		t.Fatalf("output = %q, want %q", out.String(), reader.content)
	}
	if reader.artifactID != "artifact-1" || reader.format != "md" {
		t.Fatalf("download = %q/%q, want artifact-1/md", reader.artifactID, reader.format)
	}
}

func TestArtifactExportWriterRejectsUnavailableArtifact(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		artifact *pb.Artifact
		want     string
	}{
		{"failed", &pb.Artifact{ArtifactId: "failed", Type: pb.ArtifactType_ARTIFACT_TYPE_10, State: pb.ArtifactState_ARTIFACT_STATE_FAILED}, "not READY"},
		{"creating", &pb.Artifact{ArtifactId: "creating", Type: pb.ArtifactType_ARTIFACT_TYPE_10, State: pb.ArtifactState_ARTIFACT_STATE_CREATING}, "not READY"},
		{"native type 9", &pb.Artifact{ArtifactId: "cards", Type: pb.ArtifactType_ARTIFACT_TYPE_9, State: pb.ArtifactState_ARTIFACT_STATE_READY}, "not supported"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reader := new(fakeArtifactFileReader)
			_, err := artifactExportWriter(reader, test.artifact, "md")
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want substring %q", err, test.want)
			}
			if !errors.Is(err, errPrecondition) {
				t.Fatalf("error = %v, want errPrecondition", err)
			}
			if reader.called {
				t.Fatal("download called for unavailable artifact")
			}
		})
	}
}

type fakeArtifactFileReader struct {
	content    string
	artifactID string
	format     string
	called     bool
}

func (f *fakeArtifactFileReader) ReadArtifactFile(_ context.Context, artifactID, format string, w io.Writer) error {
	f.called = true
	f.artifactID = artifactID
	f.format = format
	_, err := io.WriteString(w, f.content)
	return err
}

func appArtifact(data string) *pb.Artifact {
	return &pb.Artifact{
		ArtifactId: "artifact-1",
		Type:       pb.ArtifactType_ARTIFACT_TYPE_REPORT,
		TailoredReport: &pb.ArtifactReportConfig{
			MindMapDataJson: data,
		},
	}
}
