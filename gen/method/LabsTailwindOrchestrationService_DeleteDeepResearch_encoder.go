package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/rpc/argbuilder"
)

// GENERATION_BEHAVIOR: append

// EncodeDeleteDeepResearchArgs encodes arguments for LabsTailwindOrchestrationService.DeleteDeepResearch
// RPC ID: LBwxtb
// Argument format: [null, [1], %conversation_id%, %project_id%]
func EncodeDeleteDeepResearchArgs(req *notebooklmv1alpha1.DeleteDeepResearchRequest) []interface{} {
	// Using generalized argument encoder. printf %q emits a properly escaped Go
	// string literal so arg_formats containing quotes (e.g. "New Note") stay valid.
	args, err := argbuilder.EncodeRPCArgs(req, "[null, [1], %conversation_id%, %project_id%]")
	if err != nil {
		// Log error and return empty args as fallback
		// In production, this should be handled better
		return []interface{}{}
	}
	return args
}
