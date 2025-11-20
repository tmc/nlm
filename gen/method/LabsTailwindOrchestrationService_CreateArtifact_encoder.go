package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/rpc/argbuilder"
)

// GENERATION_BEHAVIOR: append

// EncodeCreateArtifactArgs encodes arguments for LabsTailwindOrchestrationService.CreateArtifact
// RPC ID: xpWGLf
// Argument format: [%context%, %project_id%, %artifact%]
func EncodeCreateArtifactArgs(req *notebooklmv1alpha1.CreateArtifactRequest) []interface{} {
	// Using generalized argument encoder
	args, err := argbuilder.EncodeRPCArgs(req, "[%context%, %project_id%, %artifact%]")
	if err != nil {
		// Log error and return empty args as fallback
		// In production, this should be handled better
		return []interface{}{}
	}
	return args
}
