package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/rpc/argbuilder"
)

// GENERATION_BEHAVIOR: append

// EncodePublishGuidebookArgs encodes arguments for LabsTailwindGuidebooksService.PublishGuidebook
// RPC ID: R6smae
// Argument format: [%guidebook_id%, %settings%]
func EncodePublishGuidebookArgs(req *notebooklmv1alpha1.PublishGuidebookRequest) []interface{} {
	// Using generalized argument encoder
	args, err := argbuilder.EncodeRPCArgs(req, "[%guidebook_id%, %settings%]")
	if err != nil {
		// Log error and return empty args as fallback
		// In production, this should be handled better
		return []interface{}{}
	}
	return args
}
