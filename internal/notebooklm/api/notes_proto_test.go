package api

import (
	"testing"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/beprotojson"
)

func TestGetNotesProtoAdapterMatchesLegacyParser(t *testing.T) {
	raw := []byte(`[[["note-1",["note-1","body",[1,"157962509464",[1775436871,282578000],null,null,[1775436871,282578000],false],null,"Title","Rich",[1]]],["note-2",["note-2","",null,null,"Second","",[2]]]]]`)

	legacy, err := parseNotesResponse(raw)
	if err != nil {
		t.Fatalf("legacy parser: %v", err)
	}
	var wire pb.GetNotesWireResponse
	if err := beprotojson.Unmarshal(raw, &wire); err != nil {
		t.Fatalf("proto decoder: %v", err)
	}
	got := notesFromWireResponse(&wire)
	assertEquivalent(t, "notes adaptation", legacy, got)
}

func TestNotesFromWireResponseNilAndEmpty(t *testing.T) {
	if got := notesFromWireResponse(nil); got != nil {
		t.Fatalf("nil response = %#v, want nil", got)
	}
	got := notesFromWireResponse(&pb.GetNotesWireResponse{Entries: []*pb.GetNotesEntry{nil}})
	if got == nil || len(got) != 0 {
		t.Fatalf("empty entries = %#v, want non-nil empty slice", got)
	}
}
