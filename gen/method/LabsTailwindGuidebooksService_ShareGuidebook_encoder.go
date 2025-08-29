package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeShareGuidebookArgs encodes arguments for LabsTailwindGuidebooksService.ShareGuidebook
// RPC ID: OTl0K
// Argument format: [%guidebook_id%, %settings%]
func EncodeShareGuidebookArgs(req *notebooklmv1alpha1.ShareGuidebookRequest) []interface{} {
	return []interface{}{
		req.GetGuidebookId(),
		req.GetSettings(),
	}
}
