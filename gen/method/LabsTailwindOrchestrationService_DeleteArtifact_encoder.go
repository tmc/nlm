package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/rpc/argbuilder"
)

// GENERATION_BEHAVIOR: append

// EncodeDeleteArtifactArgs encodes arguments for LabsTailwindOrchestrationService.DeleteArtifact
// RPC ID: WxBZtb
// Argument format: [%artifact_id%]
func EncodeDeleteArtifactArgs(req *notebooklmv1alpha1.DeleteArtifactRequest) []interface{} {
	// Using generalized argument encoder
	args, err := argbuilder.EncodeRPCArgs(req, "[%artifact_id%]")
	if err != nil {
		// Log error and return empty args as fallback
		// In production, this should be handled better
		return []interface{}{}
	}
	return args
}
