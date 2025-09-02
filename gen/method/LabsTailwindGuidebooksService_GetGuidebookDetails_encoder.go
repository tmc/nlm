package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/rpc/argbuilder"
)

// GENERATION_BEHAVIOR: append

// EncodeGetGuidebookDetailsArgs encodes arguments for LabsTailwindGuidebooksService.GetGuidebookDetails
// RPC ID: LJyzeb
// Argument format: [%guidebook_id%]
func EncodeGetGuidebookDetailsArgs(req *notebooklmv1alpha1.GetGuidebookDetailsRequest) []interface{} {
	// Using generalized argument encoder
	args, err := argbuilder.EncodeRPCArgs(req, "[%guidebook_id%]")
	if err != nil {
		// Log error and return empty args as fallback
		// In production, this should be handled better
		return []interface{}{}
	}
	return args
}
