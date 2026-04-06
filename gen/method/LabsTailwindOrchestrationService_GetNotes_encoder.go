package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeGetNotesArgs encodes arguments for LabsTailwindOrchestrationService.GetNotes
// RPC ID: cFji9
//
// Wire format (from HAR): [project_id, null, [timestamp_seconds, timestamp_nanos], [2]]
// Simplified (omit timestamp for full list): [project_id, null, null, [2]]
func EncodeGetNotesArgs(req *notebooklmv1alpha1.GetNotesRequest) []interface{} {
	return []interface{}{
		req.GetProjectId(),
		nil,
		nil,
		[]interface{}{2},
	}
}
