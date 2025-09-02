package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/rpc/argbuilder"
)

// GENERATION_BEHAVIOR: append

// EncodeLoadSourceArgs encodes arguments for LabsTailwindOrchestrationService.LoadSource
// RPC ID: hizoJc
// Argument format: [%source_id%]
func EncodeLoadSourceArgs(req *notebooklmv1alpha1.LoadSourceRequest) []interface{} {
	// Using generalized argument encoder
	args, err := argbuilder.EncodeRPCArgs(req, "[%source_id%]")
	if err != nil {
		// Log error and return empty args as fallback
		// In production, this should be handled better
		return []interface{}{}
	}
	return args
}
