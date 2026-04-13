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
//	[null, null, [noteId1, noteId2, ...], [2]]
//
// Note: project_id is passed via URL source-path, NOT in the args array.
func EncodeDeleteNotesArgs(req *notebooklmv1alpha1.DeleteNotesRequest) []interface{} {
	noteIDs := make([]interface{}, len(req.GetNoteIds()))
	for i, id := range req.GetNoteIds() {
		noteIDs[i] = id
	}

	return []interface{}{
		nil,              // pos 0: project ID (injected by caller via source-path)
		nil,              // pos 1: null
		noteIDs,          // pos 2: [noteId1, ...]
		[]interface{}{2}, // pos 3: ProjectContext
	}
}
