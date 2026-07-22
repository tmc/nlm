package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/rpc/argbuilder"
)

// GENERATION_BEHAVIOR: append

// EncodeFetchInteractivityTokenArgs encodes arguments for LabsTailwindOrchestrationService.FetchInteractivityToken
// RPC ID: Of0kDd
// Argument format: [[%context_version%, null, %context_surface%, %context_caps%]]
func EncodeFetchInteractivityTokenArgs(req *notebooklmv1alpha1.FetchInteractivityTokenRequest) []interface{} {
	// Using generalized argument encoder. printf %q emits a properly escaped Go
	// string literal so arg_formats containing quotes (e.g. "New Note") stay valid.
	args, err := argbuilder.EncodeRPCArgs(req, "[[%context_version%, null, %context_surface%, %context_caps%]]")
	if err != nil {
		// Log error and return empty args as fallback
		// In production, this should be handled better
		return []interface{}{}
	}
	return args
}
