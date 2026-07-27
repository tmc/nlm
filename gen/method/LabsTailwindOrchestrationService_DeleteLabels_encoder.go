package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/argbuilder"
)

// GENERATION_BEHAVIOR: append

// EncodeDeleteLabelsArgs encodes arguments for LabsTailwindOrchestrationService.DeleteLabels
// RPC ID: GyzE7e
// Argument format: [[2], %project_id%, %label_ids%]
func EncodeDeleteLabelsArgs(req *notebooklmv1alpha1.DeleteLabelsRequest) []interface{} {
	// Using generalized argument encoder. printf %q emits a properly escaped Go
	// string literal so arg_formats containing quotes (e.g. "New Note") stay valid.
	args, err := argbuilder.EncodeRPCArgs(req, "[[2], %project_id%, %label_ids%]")
	if err != nil {
		// Log error and return empty args as fallback
		// In production, this should be handled better
		return []interface{}{}
	}
	return args
}
