package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/rpc/argbuilder"
)

// GENERATION_BEHAVIOR: append

// EncodeCreateLabelArgs encodes arguments for LabsTailwindOrchestrationService.CreateLabel
// RPC ID: agX4Bc
// Argument format: [%context%, %project_id%, null, null, null, %creation%, %scope%]
func EncodeCreateLabelArgs(req *notebooklmv1alpha1.CreateLabelRequest) []interface{} {
	// Using generalized argument encoder. printf %q emits a properly escaped Go
	// string literal so arg_formats containing quotes (e.g. "New Note") stay valid.
	args, err := argbuilder.EncodeRPCArgs(req, "[%context%, %project_id%, null, null, null, %creation%, %scope%]")
	if err != nil {
		// Log error and return empty args as fallback
		// In production, this should be handled better
		return []interface{}{}
	}
	return args
}
