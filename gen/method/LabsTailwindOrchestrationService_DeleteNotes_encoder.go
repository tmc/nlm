package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeDeleteNotesArgs encodes arguments for LabsTailwindOrchestrationService.DeleteNotes
// RPC ID: AH0mwd
// Argument format: [%note_ids%]
func EncodeDeleteNotesArgs(req *notebooklmv1alpha1.DeleteNotesRequest) []interface{} {
	var noteIds []interface{}
	for _, noteId := range req.GetNoteIds() {
		noteIds = append(noteIds, noteId)
	}
	return []interface{}{noteIds}
}
