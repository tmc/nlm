package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeMutateAccountArgs encodes arguments for LabsTailwindOrchestrationService.MutateAccount
// RPC ID: hT54vc
// Argument format: [%account%, %update_mask%]
func EncodeMutateAccountArgs(req *notebooklmv1alpha1.MutateAccountRequest) []interface{} {
	return []interface{}{
		req.GetAccount(),
		req.GetUpdateMask(),
	}
}
