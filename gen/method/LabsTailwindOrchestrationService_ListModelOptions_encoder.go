package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/rpc/argbuilder"
)

// GENERATION_BEHAVIOR: append

// EncodeListModelOptionsArgs encodes arguments for LabsTailwindOrchestrationService.ListModelOptions
// RPC ID: EnujNd
// Argument format: [[%version%, null, %surface%, %caps%]]
func EncodeListModelOptionsArgs(req *notebooklmv1alpha1.ListModelOptionsRequest) []interface{} {
	// Using generalized argument encoder. printf %q emits a properly escaped Go
	// string literal so arg_formats containing quotes (e.g. "New Note") stay valid.
	args, err := argbuilder.EncodeRPCArgs(req, "[[%version%, null, %surface%, %caps%]]")
	if err != nil {
		// Log error and return empty args as fallback
		// In production, this should be handled better
		return []interface{}{}
	}
	return args
}
