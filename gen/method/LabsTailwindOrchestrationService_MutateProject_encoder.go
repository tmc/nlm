package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/rpc/argbuilder"
)

// GENERATION_BEHAVIOR: append

// EncodeMutateProjectArgs encodes arguments for LabsTailwindOrchestrationService.MutateProject
// RPC ID: s0tc2d
// Argument format: [%project_id%, %updates%]
func EncodeMutateProjectArgs(req *notebooklmv1alpha1.MutateProjectRequest) []interface{} {
	// Using generalized argument encoder
	args, err := argbuilder.EncodeRPCArgs(req, "[%project_id%, %updates%]")
	if err != nil {
		// Log error and return empty args as fallback
		// In production, this should be handled better
		return []interface{}{}
	}
	return args
}
