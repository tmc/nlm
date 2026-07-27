package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/rpc/argbuilder"
)

// GENERATION_BEHAVIOR: append

// EncodeCreateUniversalArtifactArgs encodes arguments for LabsTailwindOrchestrationService.CreateUniversalArtifact
// RPC ID: R7cb6c
// Argument format: [%context%, %project_id%, %options%, null, null, %unknown_6%]
func EncodeCreateUniversalArtifactArgs(req *notebooklmv1alpha1.CreateUniversalArtifactRequest) []interface{} {
	// Using generalized argument encoder. printf %q emits a properly escaped Go
	// string literal so arg_formats containing quotes (e.g. "New Note") stay valid.
	args, err := argbuilder.EncodeRPCArgs(req, "[%context%, %project_id%, %options%, null, null, %unknown_6%]")
	if err != nil {
		// Log error and return empty args as fallback
		// In production, this should be handled better
		return []interface{}{}
	}
	if req.GetUnknown_6() == nil {
		return args[:3]
	}
	return args
}
