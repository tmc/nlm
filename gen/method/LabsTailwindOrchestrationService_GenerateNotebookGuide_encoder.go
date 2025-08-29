package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeGenerateNotebookGuideArgs encodes arguments for LabsTailwindOrchestrationService.GenerateNotebookGuide
// RPC ID: VfAZjd
// Argument format: [%project_id%]
func EncodeGenerateNotebookGuideArgs(req *notebooklmv1alpha1.GenerateNotebookGuideRequest) []interface{} {
	// Single project ID encoding
	return []interface{}{req.GetProjectId()}
}
