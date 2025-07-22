package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeDeleteProjectsArgs encodes arguments for LabsTailwindOrchestrationService.DeleteProjects
// RPC ID: WWINqb
// Argument format: [%project_ids%]
func EncodeDeleteProjectsArgs(req *notebooklmv1alpha1.DeleteProjectsRequest) []interface{} {
	// Multiple project IDs encoding
	return []interface{}{req.GetProjectIds()}
}
