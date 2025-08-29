package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeGetGuidebookArgs encodes arguments for LabsTailwindGuidebooksService.GetGuidebook
// RPC ID: EYqtU
// Argument format: [%guidebook_id%]
func EncodeGetGuidebookArgs(req *notebooklmv1alpha1.GetGuidebookRequest) []interface{} {
	return []interface{}{
		req.GetGuidebookId(),
	}
}
