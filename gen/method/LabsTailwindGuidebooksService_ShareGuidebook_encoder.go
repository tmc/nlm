package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/rpc/argbuilder"
)

// GENERATION_BEHAVIOR: append

// EncodeShareGuidebookArgs encodes arguments for LabsTailwindGuidebooksService.ShareGuidebook
// RPC ID: OTl0K
// Argument format: [%guidebook_id%, %settings%]
func EncodeShareGuidebookArgs(req *notebooklmv1alpha1.ShareGuidebookRequest) []interface{} {
	// Using generalized argument encoder
	args, err := argbuilder.EncodeRPCArgs(req, "[%guidebook_id%, %settings%]")
	if err != nil {
		// Log error and return empty args as fallback
		// In production, this should be handled better
		return []interface{}{}
	}
	return args
}
