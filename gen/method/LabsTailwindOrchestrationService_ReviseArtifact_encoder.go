package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/argbuilder"
)

// GENERATION_BEHAVIOR: append

// EncodeReviseArtifactArgs encodes arguments for LabsTailwindOrchestrationService.ReviseArtifact
// RPC ID: KmcKPe
// Argument format: [%context%, %artifact_id%, %instructions%]
func EncodeReviseArtifactArgs(req *notebooklmv1alpha1.ReviseArtifactRequest) []interface{} {
	// Using generalized argument encoder. printf %q emits a properly escaped Go
	// string literal so arg_formats containing quotes (e.g. "New Note") stay valid.
	args, err := argbuilder.EncodeRPCArgs(req, "[%context%, %artifact_id%, %instructions%]")
	if err != nil {
		// Log error and return empty args as fallback
		// In production, this should be handled better
		return []interface{}{}
	}
	return args
}
