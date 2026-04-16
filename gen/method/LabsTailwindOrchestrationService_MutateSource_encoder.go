package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/rpc/argbuilder"
)

// GENERATION_BEHAVIOR: append

// EncodeMutateSourceArgs encodes arguments for LabsTailwindOrchestrationService.MutateSource
// RPC ID: b7Wfje
// Argument format: [%source_id%, %updates%]
func EncodeMutateSourceArgs(req *notebooklmv1alpha1.MutateSourceRequest) []interface{} {
	// Using generalized argument encoder
	args, err := argbuilder.EncodeRPCArgs(req, "[%source_id%, %updates%]")
	if err != nil {
		// Log error and return empty args as fallback
		// In production, this should be handled better
		return []interface{}{}
	}
	return args
}
