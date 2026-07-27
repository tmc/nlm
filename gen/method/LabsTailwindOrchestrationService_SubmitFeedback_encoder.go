package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/argbuilder"
)

// GENERATION_BEHAVIOR: append

// EncodeSubmitFeedbackArgs encodes arguments for LabsTailwindOrchestrationService.SubmitFeedback
// RPC ID: uNyJKe
// Argument format: [%turn%, %feedback%, %content%, null, null, %options%]
func EncodeSubmitFeedbackArgs(req *notebooklmv1alpha1.SubmitFeedbackRequest) []interface{} {
	// Using generalized argument encoder. printf %q emits a properly escaped Go
	// string literal so arg_formats containing quotes (e.g. "New Note") stay valid.
	args, err := argbuilder.EncodeRPCArgs(req, "[%turn%, %feedback%, %content%, null, null, %options%]")
	if err != nil {
		// Log error and return empty args as fallback
		// In production, this should be handled better
		return []interface{}{}
	}
	return args
}
