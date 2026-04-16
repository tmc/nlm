package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/rpc/argbuilder"
)

// GENERATION_BEHAVIOR: append

// EncodeDeleteSourcesArgs encodes arguments for LabsTailwindOrchestrationService.DeleteSources
// RPC ID: tGMBJ
// Argument format: [[%source_ids%]]
func EncodeDeleteSourcesArgs(req *notebooklmv1alpha1.DeleteSourcesRequest) []interface{} {
	// Using generalized argument encoder
	args, err := argbuilder.EncodeRPCArgs(req, "[[%source_ids%]]")
	if err != nil {
		// Log error and return empty args as fallback
		// In production, this should be handled better
		return []interface{}{}
	}
	return args
}
