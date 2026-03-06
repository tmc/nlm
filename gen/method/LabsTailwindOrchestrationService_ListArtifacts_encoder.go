package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/rpc/argbuilder"
)

// GENERATION_BEHAVIOR: append

// EncodeListArtifactsArgs encodes arguments for LabsTailwindOrchestrationService.ListArtifacts
// RPC ID: LfTXoe
// Argument format: [%project_id%, %page_size%, %page_token%]
func EncodeListArtifactsArgs(req *notebooklmv1alpha1.ListArtifactsRequest) []interface{} {
	// Using generalized argument encoder
	args, err := argbuilder.EncodeRPCArgs(req, "[%project_id%, %page_size%, %page_token%]")
	if err != nil {
		// Log error and return empty args as fallback
		// In production, this should be handled better
		return []interface{}{}
	}
	return args
}
