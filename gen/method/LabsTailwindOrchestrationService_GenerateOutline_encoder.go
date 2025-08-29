package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeGenerateOutlineArgs encodes arguments for LabsTailwindOrchestrationService.GenerateOutline
// RPC ID: lCjAd
// Argument format: [%project_id%]
func EncodeGenerateOutlineArgs(req *notebooklmv1alpha1.GenerateOutlineRequest) []interface{} {
	// Single project ID encoding
	return []interface{}{req.GetProjectId()}
}
