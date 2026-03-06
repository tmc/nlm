package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/rpc/argbuilder"
)

// GENERATION_BEHAVIOR: append

// EncodeUpdateArtifactArgs encodes arguments for LabsTailwindOrchestrationService.UpdateArtifact
// RPC ID: DJezBc
// Argument format: [%artifact%, %update_mask%]
func EncodeUpdateArtifactArgs(req *notebooklmv1alpha1.UpdateArtifactRequest) []interface{} {
	// Using generalized argument encoder
	args, err := argbuilder.EncodeRPCArgs(req, "[%artifact%, %update_mask%]")
	if err != nil {
		// Log error and return empty args as fallback
		// In production, this should be handled better
		return []interface{}{}
	}
	return args
}
