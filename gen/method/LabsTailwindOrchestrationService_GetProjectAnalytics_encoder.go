package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeGetProjectAnalyticsArgs encodes arguments for LabsTailwindOrchestrationService.GetProjectAnalytics
// RPC ID: AUrzMb
// Argument format: [%project_id%]
func EncodeGetProjectAnalyticsArgs(req *notebooklmv1alpha1.GetProjectAnalyticsRequest) []interface{} {
	// Single project ID encoding
	return []interface{}{req.GetProjectId()}
}
