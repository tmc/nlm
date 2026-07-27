package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/rpc/argbuilder"
)

// GENERATION_BEHAVIOR: append

// EncodeStartFastResearchArgs encodes arguments for LabsTailwindOrchestrationService.StartFastResearch
// RPC ID: Ljjv0c
// Argument format: [[%query%, 1], null, 1, %project_id%]
func EncodeStartFastResearchArgs(req *notebooklmv1alpha1.StartFastResearchRequest) []interface{} {
	// Using generalized argument encoder. printf %q emits a properly escaped Go
	// string literal so arg_formats containing quotes (e.g. "New Note") stay valid.
	args, err := argbuilder.EncodeRPCArgs(req, "[[%query%, 1], null, 1, %project_id%]")
	if err != nil {
		// Log error and return empty args as fallback
		// In production, this should be handled better
		return []interface{}{}
	}
	return args
}
