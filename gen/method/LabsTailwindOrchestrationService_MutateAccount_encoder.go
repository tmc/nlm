package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/rpc/argbuilder"
)

// GENERATION_BEHAVIOR: append

// EncodeMutateAccountArgs encodes arguments for LabsTailwindOrchestrationService.MutateAccount
// RPC ID: hT54vc
// Argument format: [%account%, %update_mask%]
func EncodeMutateAccountArgs(req *notebooklmv1alpha1.MutateAccountRequest) []interface{} {
	// Using generalized argument encoder
	args, err := argbuilder.EncodeRPCArgs(req, "[%account%, %update_mask%]")
	if err != nil {
		// Log error and return empty args as fallback
		// In production, this should be handled better
		return []interface{}{}
	}
	return args
}
