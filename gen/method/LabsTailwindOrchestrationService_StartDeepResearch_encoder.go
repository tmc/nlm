package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/argbuilder"
)

// GENERATION_BEHAVIOR: append

// EncodeStartDeepResearchArgs encodes arguments for LabsTailwindOrchestrationService.StartDeepResearch
// RPC ID: QA9ei
// Argument format: [null, [1], [%query%, 1], 5, %project_id%]
func EncodeStartDeepResearchArgs(req *notebooklmv1alpha1.StartDeepResearchRequest) []interface{} {
	// Using generalized argument encoder. printf %q emits a properly escaped Go
	// string literal so arg_formats containing quotes (e.g. "New Note") stay valid.
	args, err := argbuilder.EncodeRPCArgs(req, "[null, [1], [%query%, 1], 5, %project_id%]")
	if err != nil {
		// Log error and return empty args as fallback
		// In production, this should be handled better
		return []interface{}{}
	}
	return args
}
