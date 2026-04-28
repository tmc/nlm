package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeCreateNoteArgs encodes arguments for LabsTailwindOrchestrationService.CreateNote
// RPC ID: CYK0Xb
//
// HAR-verified wire format (2026-04-28):
//
//	[project_id, "", [1], null, "New Note", null, [2]]
//
// CreateNote produces an empty "New Note" shell. Title and body are NOT passed
// here — the web UI follows up with a MutateNote (cYAfTb) to set them. Use
// the high-level api.Client.CreateNote for the chained operation.
func EncodeCreateNoteArgs(req *notebooklmv1alpha1.CreateNoteRequest) []interface{} {
	return []interface{}{
		req.GetProjectId(),
		"",
		[]interface{}{1},
		nil,
		"New Note",
		nil,
		[]interface{}{2},
	}
}
