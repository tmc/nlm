package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/argbuilder"
)

// GENERATION_BEHAVIOR: append

// EncodeGenerateMagicViewArgs encodes arguments for LabsTailwindOrchestrationService.GenerateMagicView
// RPC ID: uK8f7c
// Argument format: [%context%, %project_id%]
func EncodeGenerateMagicViewArgs(req *notebooklmv1alpha1.GenerateMagicViewRequest) []interface{} {
	// Using generalized argument encoder. printf %q emits a properly escaped Go
	// string literal so arg_formats containing quotes (e.g. "New Note") stay valid.
	args, err := argbuilder.EncodeRPCArgs(req, "[%context%, %project_id%]")
	if err != nil {
		// Log error and return empty args as fallback
		// In production, this should be handled better
		return []interface{}{}
	}
	return args
}
