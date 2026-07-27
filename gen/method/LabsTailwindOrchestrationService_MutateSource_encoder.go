package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/argbuilder"
)

// GENERATION_BEHAVIOR: append

// EncodeMutateSourceArgs encodes arguments for LabsTailwindOrchestrationService.MutateSource
// RPC ID: b7Wfje
// Argument format: [null, %source_id%, %updates%, [2, null, [1], [1, null, null, null, null, null, null, null, null, null, [1, 3]]]]
func EncodeMutateSourceArgs(req *notebooklmv1alpha1.MutateSourceRequest) []interface{} {
	// Using generalized argument encoder. printf %q emits a properly escaped Go
	// string literal so arg_formats containing quotes (e.g. "New Note") stay valid.
	args, err := argbuilder.EncodeRPCArgs(req, "[null, %source_id%, %updates%, [2, null, [1], [1, null, null, null, null, null, null, null, null, null, [1, 3]]]]")
	if err != nil {
		// Log error and return empty args as fallback
		// In production, this should be handled better
		return []interface{}{}
	}
	return args
}
