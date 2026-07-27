package api

import (
	"testing"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/beprotojson"
)

func TestGetNotesProtoAdapterProjection(t *testing.T) {
	raw := []byte(`[[["note-1",["note-1","body",[1,"157962509464",[1775436871,282578000],null,null,[1775436871,282578000],false],null,"Title","Rich",[1]]],["note-2",["note-2","",null,null,"Second","",[2]]]]]`)
	var wire pb.GetNotesRichWireResponse
	if err := beprotojson.Unmarshal(raw, &wire); err != nil {
		t.Fatalf("proto decoder: %v", err)
	}
	got := notesFromWireResponse(&wire)
	want := []*pb.Note{
		{NoteId: "note-1", ContentText: "body", Title: "Title", RichText: "Rich"},
		{NoteId: "note-2", Title: "Second"},
	}
	assertEquivalent(t, "notes adaptation", want, got)
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
