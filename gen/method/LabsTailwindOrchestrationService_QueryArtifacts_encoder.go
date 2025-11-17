package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/rpc/argbuilder"
)

// GENERATION_BEHAVIOR: append

// EncodeQueryArtifactsArgs encodes arguments for LabsTailwindOrchestrationService.QueryArtifacts
// RPC ID: gArtLc
// Argument format: [%artifact_types%, %project_id%, %filter%]
func EncodeQueryArtifactsArgs(req *notebooklmv1alpha1.QueryArtifactsRequest) []interface{} {
	// Using generalized argument encoder
	args, err := argbuilder.EncodeRPCArgs(req, "[%artifact_types%, %project_id%, %filter%]")
	if err != nil {
		// Log error and return empty args as fallback
		// In production, this should be handled better
		return []interface{}{}
	}
	return args
}
