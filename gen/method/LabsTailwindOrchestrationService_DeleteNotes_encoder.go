package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeDeleteNotesArgs encodes arguments for LabsTailwindOrchestrationService.DeleteNotes
// RPC ID: AH0mwd
// Wire format verified against HAR capture — do not regenerate.
//
// HAR-verified wire format:
//
//	[projectId, null, [noteId1, noteId2, ...], [2]]
func EncodeDeleteNotesArgs(req *notebooklmv1alpha1.DeleteNotesRequest) []interface{} {
	noteIDs := make([]interface{}, len(req.GetNoteIds()))
	for i, id := range req.GetNoteIds() {
		noteIDs[i] = id
	}

	return []interface{}{
		req.GetProjectId(), // pos 0: project ID
		nil,                // pos 1: null
		noteIDs,            // pos 2: [noteId1, ...]
		[]interface{}{2},   // pos 3: ProjectContext
	}
}
