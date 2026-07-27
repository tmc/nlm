package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/rpc/argbuilder"
)

// GENERATION_BEHAVIOR: append

// EncodeMutateLabelArgs encodes arguments for LabsTailwindOrchestrationService.MutateLabel
// RPC ID: le8sX
// Argument format: [%context%, %project_id%, %label_id%, %mutation%]
func EncodeMutateLabelArgs(req *notebooklmv1alpha1.MutateLabelRequest) []interface{} {
	// Using generalized argument encoder. printf %q emits a properly escaped Go
	// string literal so arg_formats containing quotes (e.g. "New Note") stay valid.
	args, err := argbuilder.EncodeRPCArgs(req, "[%context%, %project_id%, %label_id%, %mutation%]")
	if err != nil {
		// Log error and return empty args as fallback
		// In production, this should be handled better
		return []interface{}{}
	}
	return args
}
