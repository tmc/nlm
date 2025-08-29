package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeGetAudioOverviewArgs encodes arguments for LabsTailwindOrchestrationService.GetAudioOverview
// RPC ID: VUsiyb
// Argument format: [%project_id%]
func EncodeGetAudioOverviewArgs(req *notebooklmv1alpha1.GetAudioOverviewRequest) []interface{} {
	// Single project ID encoding
	return []interface{}{req.GetProjectId()}
}
