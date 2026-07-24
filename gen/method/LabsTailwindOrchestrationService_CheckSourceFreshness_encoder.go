package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/rpc/argbuilder"
)

// GENERATION_BEHAVIOR: append

// EncodeCheckSourceFreshnessArgs encodes arguments for LabsTailwindOrchestrationService.CheckSourceFreshness
// RPC ID: yR9Yof
// Argument format: [null, %source%, %context%]
func EncodeCheckSourceFreshnessArgs(req *notebooklmv1alpha1.CheckSourceFreshnessRequest) []interface{} {
	// Using generalized argument encoder. printf %q emits a properly escaped Go
	// string literal so arg_formats containing quotes (e.g. "New Note") stay valid.
	args, err := argbuilder.EncodeRPCArgs(req, "[null, %source%, %context%]")
	if err != nil {
		// Log error and return empty args as fallback
		// In production, this should be handled better
		return []interface{}{}
	}
	return args
}
