package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/rpc/argbuilder"
)

// GENERATION_BEHAVIOR: append

// EncodeCreateAudioOverviewArgs encodes arguments for LabsTailwindOrchestrationService.CreateAudioOverview
// RPC ID: AHyHrd
// Argument format: [%project_id%, %instructions%]
func EncodeCreateAudioOverviewArgs(req *notebooklmv1alpha1.CreateAudioOverviewRequest) []interface{} {
	// Using generalized argument encoder
	args, err := argbuilder.EncodeRPCArgs(req, "[%project_id%, %instructions%]")
	if err != nil {
		// Log error and return empty args as fallback
		// In production, this should be handled better
		return []interface{}{}
	}
	return args
}
