package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeGetGuidebookDetailsArgs encodes arguments for LabsTailwindGuidebooksService.GetGuidebookDetails
// RPC ID: LJyzeb
// Argument format: [%guidebook_id%]
func EncodeGetGuidebookDetailsArgs(req *notebooklmv1alpha1.GetGuidebookDetailsRequest) []interface{} {
	return []interface{}{
		req.GetGuidebookId(),
	}
}
