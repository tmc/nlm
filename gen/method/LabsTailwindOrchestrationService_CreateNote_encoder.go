package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeCreateNoteArgs encodes arguments for LabsTailwindOrchestrationService.CreateNote
// RPC ID: CYK0Xb
//
// Wire format (confirmed via HAR): [notebook_id, content, [2], null, title, null, [2]]
func EncodeCreateNoteArgs(req *notebooklmv1alpha1.CreateNoteRequest) []interface{} {
	return []interface{}{
		req.GetProjectId(),
		req.GetContent(),
		[]interface{}{2},
		nil,
		req.GetTitle(),
		nil,
		[]interface{}{2},
	}
}
