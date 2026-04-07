package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeDeleteNotesArgs encodes arguments for LabsTailwindOrchestrationService.DeleteNotes
// RPC ID: AH0mwd
//
// HAR-verified wire format:
//
//	[projectId, null, [noteId1, noteId2, ...], [2]]
//
// Note: DeleteNotesRequest proto lacks project_id field, so this encoder
// only handles the note_ids portion. The service client must set NotebookID
// separately. For now we encode without projectId at pos 0 — the service
// client wrapper adds it.
func EncodeDeleteNotesArgs(req *notebooklmv1alpha1.DeleteNotesRequest) []interface{} {
	noteIDs := make([]interface{}, len(req.GetNoteIds()))
	for i, id := range req.GetNoteIds() {
		noteIDs[i] = id
	}

	// Wire format: [null, null, [noteIds], [2]]
	// ProjectId should be injected by the caller since it's not on this proto message.
	return []interface{}{
		nil,              // pos 0: project ID (placeholder — caller must override)
		nil,              // pos 1: null
		noteIDs,          // pos 2: [noteId1, ...]
		[]interface{}{2}, // pos 3: ProjectContext
	}
}
