package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/argbuilder"
)

// GENERATION_BEHAVIOR: append

// EncodeUpdateProjectUserStateArgs encodes arguments for LabsTailwindOrchestrationService.UpdateProjectUserState
// RPC ID: LQhfEb
// Argument format: [%context%, %project_id%, %value%, %keys%]
func EncodeUpdateProjectUserStateArgs(req *notebooklmv1alpha1.UpdateProjectUserStateRequest) []interface{} {
	// Using generalized argument encoder. printf %q emits a properly escaped Go
	// string literal so arg_formats containing quotes (e.g. "New Note") stay valid.
	args, err := argbuilder.EncodeRPCArgs(req, "[%context%, %project_id%, %value%, %keys%]")
	if err != nil {
		// Log error and return empty args as fallback
		// In production, this should be handled better
		return []interface{}{}
	}
	return args
}
