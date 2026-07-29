package main

import (
	"encoding/json"
	"testing"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/notebooklm"
)

func TestNoteListRecordsFixture(t *testing.T) {
	notes := []*notebooklm.Note{
		{Note: &pb.Note{NoteId: "note-1", Title: "New Note", ContentText: "fallback\ncontent"}},
		{Note: &pb.Note{NoteId: "note-2", Title: "Rich Note", ContentText: "ignored", RichText: "**rich**\ncontent"}},
		nil,
	}
	got := noteListRecords(notes)
	want := []noteListRecord{
		{NoteID: "note-1", Title: "New Note", ContentPreview: "fallback content"},
		{NoteID: "note-2", Title: "Rich Note", ContentPreview: "**rich** content"},
	}
	gotJSON, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	if string(gotJSON) != string(wantJSON) {
		t.Fatalf("note list records = %s, want %s", gotJSON, wantJSON)
	}
}
