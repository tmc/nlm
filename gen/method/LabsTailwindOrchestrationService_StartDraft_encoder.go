package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeStartDraftArgs encodes arguments for LabsTailwindOrchestrationService.StartDraft
// RPC ID: exXvGf
// Argument format: [%project_id%]
func EncodeStartDraftArgs(req *notebooklmv1alpha1.StartDraftRequest) []interface{} {
	// Single project ID encoding
	return []interface{}{req.GetProjectId()}
}
