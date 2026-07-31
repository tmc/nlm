package render_test

import (
	"bytes"
	"strings"
	"testing"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/notebooklm"
	"github.com/tmc/nlm/render"
)

func TestSourceTextCollapsesGaps(t *testing.T) {
	// Two fragments separated by a wide offset gap: Full pads the gap with
	// spaces; SourceText collapses it to a paragraph break.
	body := notebooklm.LoadSourceText{
		Fragments: []notebooklm.TextFragment{
			{Start: 0, End: 5, Text: "first"},
			{Start: 40, End: 46, Text: "second"},
		},
	}
	full := body.Full()
	if !strings.Contains(full, "     ") {
		t.Fatalf("expected Full to pad the gap with spaces, got %q", full)
	}
	got := render.SourceText(body)
	if strings.Contains(got, "     ") {
		t.Errorf("SourceText should collapse the gap, got %q", got)
	}
	if !strings.Contains(got, "first") || !strings.Contains(got, "second") {
		t.Errorf("SourceText dropped content: %q", got)
	}
}

func TestNoteTextRendersFlatContent(t *testing.T) {
	note := &notebooklm.Note{Note: &pb.Note{
		Title:       "Sample",
		ContentText: "hello world",
	}}
	var buf bytes.Buffer
	if err := render.NoteText(&buf, note); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "hello world") {
		t.Errorf("NoteText missing content: %q", buf.String())
	}
}

func TestCitationTitlePrefersParent(t *testing.T) {
	sources := []*pb.Source{
		{SourceId: &pb.SourceId{SourceId: "parent-1"}, Title: "Parent Source"},
	}
	resolve := render.SourceTitleResolver(sources)
	// SourceID is a chunk handle absent from the source list; ParentSourceID
	// is the notebook source that resolves to a title.
	c := notebooklm.Citation{SourceID: "chunk-9", ParentSourceID: "parent-1"}
	if got := render.CitationTitle(c, resolve); got != "Parent Source" {
		t.Errorf("CitationTitle = %q, want %q", got, "Parent Source")
	}
}
