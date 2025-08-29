package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeCreateAudioOverviewArgs encodes arguments for LabsTailwindOrchestrationService.CreateAudioOverview
// RPC ID: AHyHrd
// Argument format: [%project_id%, %instructions%]
func EncodeCreateAudioOverviewArgs(req *notebooklmv1alpha1.CreateAudioOverviewRequest) []interface{} {
	return []interface{}{
		req.GetProjectId(),
		req.GetInstructions(),
	}
}
