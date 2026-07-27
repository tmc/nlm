package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/argbuilder"
)

// GENERATION_BEHAVIOR: append

// EncodeShareGuidebookArgs encodes arguments for LabsTailwindGuidebooksService.ShareGuidebook
// RPC ID: OTl0K
// Argument format: [%guidebook_id%, %settings%]
func EncodeShareGuidebookArgs(req *notebooklmv1alpha1.ShareGuidebookRequest) []interface{} {
	// Using generalized argument encoder. printf %q emits a properly escaped Go
	// string literal so arg_formats containing quotes (e.g. "New Note") stay valid.
	args, err := argbuilder.EncodeRPCArgs(req, "[%guidebook_id%, %settings%]")
	if err != nil {
		// Log error and return empty args as fallback
		// In production, this should be handled better
		return []interface{}{}
	}
	return args
}
