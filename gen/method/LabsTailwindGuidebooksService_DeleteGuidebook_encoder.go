package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeDeleteGuidebookArgs encodes arguments for LabsTailwindGuidebooksService.DeleteGuidebook
// RPC ID: ARGkVc
// Argument format: [%guidebook_id%]
func EncodeDeleteGuidebookArgs(req *notebooklmv1alpha1.DeleteGuidebookRequest) []interface{} {
	return []interface{}{
		req.GetGuidebookId(),
	}
}
