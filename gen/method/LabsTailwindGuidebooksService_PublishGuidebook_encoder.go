package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodePublishGuidebookArgs encodes arguments for LabsTailwindGuidebooksService.PublishGuidebook
// RPC ID: R6smae
// Argument format: [%guidebook_id%, %settings%]
func EncodePublishGuidebookArgs(req *notebooklmv1alpha1.PublishGuidebookRequest) []interface{} {
	return []interface{}{
		req.GetGuidebookId(),
		req.GetSettings(),
	}
}
