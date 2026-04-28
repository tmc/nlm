package method

import (
	"encoding/json"
	"testing"

	notebooklm "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// Wire shapes are pinned against a 2026-04-28 HAR capture from notebooklm.google.com:
//
//	CYK0Xb (CreateNote): [project_id, "", [1], null, "New Note", null, [2]]
//	cYAfTb (MutateNote): [project_id, note_id, [[[content, title, [tags...], 0]]], [2]]
//
// Update the want strings below when you re-capture and the server format moves.

func TestEncodeCreateNoteArgs(t *testing.T) {
	got := EncodeCreateNoteArgs(&notebooklm.CreateNoteRequest{
		ProjectId: "abc-123",
		Title:     "ignored", // server hardcodes "New Note"; title is set via MutateNote
		Content:   "ignored",
	})
	want := `["abc-123","",[1],null,"New Note",null,[2]]`
	gotJSON, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(gotJSON) != want {
		t.Fatalf("CreateNote args:\n got: %s\nwant: %s", gotJSON, want)
	}
}

func TestEncodeMutateNoteArgs(t *testing.T) {
	got := EncodeMutateNoteArgs(&notebooklm.MutateNoteRequest{
		ProjectId: "abc-123",
		NoteId:    "note-xyz",
		Updates: []*notebooklm.NoteUpdate{{
			Content: "body text",
			Title:   "the title",
			Tags:    []string{},
		}},
	})
	want := `["abc-123","note-xyz",[[["body text","the title",[],0]]],[2]]`
	gotJSON, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(gotJSON) != want {
		t.Fatalf("MutateNote args:\n got: %s\nwant: %s", gotJSON, want)
	}
}
