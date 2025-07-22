package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeRemoveRecentlyViewedProjectArgs encodes arguments for LabsTailwindOrchestrationService.RemoveRecentlyViewedProject
// RPC ID: fejl7e
// Argument format: [%project_id%]
func EncodeRemoveRecentlyViewedProjectArgs(req *notebooklmv1alpha1.RemoveRecentlyViewedProjectRequest) []interface{} {
	// Single project ID encoding
	return []interface{}{req.GetProjectId()}
}
