package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/rpc/argbuilder"
)

// GENERATION_BEHAVIOR: append

// EncodeGetGuidebookArgs encodes arguments for LabsTailwindGuidebooksService.GetGuidebook
// RPC ID: EYqtU
// Argument format: [%guidebook_id%]
func EncodeGetGuidebookArgs(req *notebooklmv1alpha1.GetGuidebookRequest) []interface{} {
	// Using generalized argument encoder
	args, err := argbuilder.EncodeRPCArgs(req, "[%guidebook_id%]")
	if err != nil {
		// Log error and return empty args as fallback
		// In production, this should be handled better
		return []interface{}{}
	}
	return args
}
