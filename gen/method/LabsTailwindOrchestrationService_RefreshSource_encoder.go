package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/argbuilder"
)

// GENERATION_BEHAVIOR: append

// EncodeRefreshSourceArgs encodes arguments for LabsTailwindOrchestrationService.RefreshSource
// RPC ID: FLmJqe
// Argument format: [null, %source%, %context%]
func EncodeRefreshSourceArgs(req *notebooklmv1alpha1.RefreshSourceRequest) []interface{} {
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
