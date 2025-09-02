package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/rpc/argbuilder"
)

// GENERATION_BEHAVIOR: append

// EncodeGetOrCreateAccountArgs encodes arguments for LabsTailwindOrchestrationService.GetOrCreateAccount
// RPC ID: ZwVcOc
// Argument format: []
func EncodeGetOrCreateAccountArgs(req *notebooklmv1alpha1.GetOrCreateAccountRequest) []interface{} {
	// Using generalized argument encoder
	args, err := argbuilder.EncodeRPCArgs(req, "[]")
	if err != nil {
		// Log error and return empty args as fallback
		// In production, this should be handled better
		return []interface{}{}
	}
	return args
}
