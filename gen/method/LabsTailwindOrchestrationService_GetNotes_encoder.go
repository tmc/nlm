package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeGetNotesArgs encodes arguments for LabsTailwindOrchestrationService.GetNotes
// RPC ID: cFji9
// Argument format: [%project_id%]
func EncodeGetNotesArgs(req *notebooklmv1alpha1.GetNotesRequest) []interface{} {
	// Single project ID encoding
	return []interface{}{req.GetProjectId()}
}
