package notebooklm

import (
	"testing"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/beprotojson"
	"google.golang.org/protobuf/proto"
)

func TestGetNotesProtoAdapterProjection(t *testing.T) {
	raw := []byte(`[[["note-1",["note-1","body",[1,"157962509464",[1775436871,282578000],null,null,[1775436871,282578000],false],null,"Title","Rich",[1]]],["note-2",["note-2","",null,null,"Second","",[2]]]]]`)
	var wire pb.GetNotesRichWireResponse
	if err := beprotojson.Unmarshal(raw, &wire); err != nil {
		t.Fatalf("proto decoder: %v", err)
	}
	got := notesFromWireResponse(&wire)
	want := []*Note{
		{Note: &pb.Note{NoteId: "note-1", ContentText: "body", Title: "Title", RichText: "Rich"}},
		{Note: &pb.Note{NoteId: "note-2", Title: "Second"}},
	}
	assertEquivalent(t, "notes adaptation", want, got)
}

func TestNoteFromRecordPreservesRichDocument(t *testing.T) {
	document := &pb.RichDocument{
		Body: &pb.SpanLayers{
			Blocks: []*pb.Span{{Start: proto.Int64(0), End: proto.Int64(4)}},
		},
	}
	grounding := []*pb.Grounding{{Score: proto.Float64(0.75)}}
	got := noteFromRecord(&pb.GetNotesRichRecord{
		NoteId:           "note-1",
		ContentText:      proto.String("body"),
		Title:            "Title",
		RichText:         &pb.NoteRichText{Value: &pb.NoteRichText_Document{Document: document}},
		GroundingDetails: &pb.NoteGroundingDetails{Grounding: grounding},
	})
	if got.GetRichText() != "" {
		t.Fatalf("RichText = %q, want empty for document arm", got.GetRichText())
	}
	if got.Rich != document {
		t.Fatal("Rich does not preserve decoded document")
	}
	if len(got.Grounding) != 1 || got.Grounding[0] != grounding[0] {
		t.Fatal("Grounding does not preserve decoded details")
	}
}

func TestNotesFromWireResponseNilAndEmpty(t *testing.T) {
	if got := notesFromWireResponse(nil); got != nil {
		t.Fatalf("nil response = %#v, want nil", got)
	}
	got := notesFromWireResponse(&pb.GetNotesRichWireResponse{Entries: []*pb.GetNotesRichEntry{nil}})
	if got == nil || len(got) != 0 {
		t.Fatalf("empty entries = %#v, want non-nil empty slice", got)
	}
}

func TestNotesFromWireResponseOmitsTombstones(t *testing.T) {
	response := &pb.GetNotesRichWireResponse{Entries: []*pb.GetNotesRichEntry{
		{NoteId: "deleted"},
		{NoteId: "present", Note: &pb.GetNotesRichRecord{NoteId: "present", Title: "Present"}},
	}}
	got := notesFromWireResponse(response)
	if len(got) != 1 || got[0].GetNoteId() != "present" {
		t.Fatalf("notes = %#v, want only present note", got)
	}
}

func TestNotesFromArtifacts(t *testing.T) {
	artifacts := []*pb.Artifact{
		{
			ArtifactId: "artifact-note",
			Title:      "Generated note",
			Type:       pb.ArtifactType_ARTIFACT_TYPE_NOTE,
			Note: &pb.ArtifactNotePreview{Config: &pb.ArtifactNoteConfig{
				Prompt: "Summarize the sources.",
			}},
		},
		{ArtifactId: "report", Type: pb.ArtifactType_ARTIFACT_TYPE_REPORT},
		nil,
	}
	got := notesFromArtifacts(artifacts)
	want := []*Note{{Note: &pb.Note{
		NoteId:      "artifact-note",
		Title:       "Generated note",
		ContentText: "Summarize the sources.",
	}}}
	assertEquivalent(t, "artifact notes", want, got)
}

func TestMergeNotesPrefersNotesRPC(t *testing.T) {
	notes := []*Note{{Note: &pb.Note{NoteId: "shared", Title: "Editable"}}}
	artifactNotes := []*Note{
		{Note: &pb.Note{NoteId: "shared", Title: "Artifact"}},
		{Note: &pb.Note{NoteId: "artifact-only", Title: "Generated"}},
	}
	got := mergeNotes(notes, artifactNotes)
	if len(got) != 2 {
		t.Fatalf("merged notes = %d, want 2", len(got))
	}
	if got[0].GetTitle() != "Editable" {
		t.Fatalf("shared note title = %q, want Editable", got[0].GetTitle())
	}
	if got[1].GetNoteId() != "artifact-only" {
		t.Fatalf("second note ID = %q, want artifact-only", got[1].GetNoteId())
	}
}
