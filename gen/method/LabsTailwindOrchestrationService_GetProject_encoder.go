package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeGetProjectArgs encodes arguments for LabsTailwindOrchestrationService.GetProject
// RPC ID: rLM1Ne
// Argument format: [%project_id%]
func EncodeGetProjectArgs(req *notebooklmv1alpha1.GetProjectRequest) []interface{} {
	// Single project ID encoding
	return []interface{}{req.GetProjectId()}
}
