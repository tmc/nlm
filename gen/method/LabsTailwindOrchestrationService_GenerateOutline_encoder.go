package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/rpc/argbuilder"
)

// GENERATION_BEHAVIOR: append

// EncodeGenerateOutlineArgs encodes arguments for LabsTailwindOrchestrationService.GenerateOutline
// RPC ID: lCjAd
// Argument format: [%project_id%]
func EncodeGenerateOutlineArgs(req *notebooklmv1alpha1.GenerateOutlineRequest) []interface{} {
	// Using generalized argument encoder
	args, err := argbuilder.EncodeRPCArgs(req, "[%project_id%]")
	if err != nil {
		// Log error and return empty args as fallback
		// In production, this should be handled better
		return []interface{}{}
	}
	return args
}
