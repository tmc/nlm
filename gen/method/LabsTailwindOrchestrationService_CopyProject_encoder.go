package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/argbuilder"
)

// GENERATION_BEHAVIOR: append

// EncodeCopyProjectArgs encodes arguments for LabsTailwindOrchestrationService.CopyProject
// RPC ID: te3DCe
// Argument format: [%context%, %source_project_id%, %new_title%]
func EncodeCopyProjectArgs(req *notebooklmv1alpha1.CopyProjectRequest) []interface{} {
	// Using generalized argument encoder. printf %q emits a properly escaped Go
	// string literal so arg_formats containing quotes (e.g. "New Note") stay valid.
	args, err := argbuilder.EncodeRPCArgs(req, "[%context%, %source_project_id%, %new_title%]")
	if err != nil {
		// Log error and return empty args as fallback
		// In production, this should be handled better
		return []interface{}{}
	}
	return args
}
