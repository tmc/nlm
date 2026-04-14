package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodePublishGuidebookArgs encodes arguments for LabsTailwindGuidebooksService.PublishGuidebook
// RPC ID: R6smae
// Wire format verified against HAR capture (khqZz):
//   [[], null, null, "<guidebook_id>", 20]
func EncodePublishGuidebookArgs(req *notebooklmv1alpha1.PublishGuidebookRequest) []interface{} {
	return EncodePublishGuidebookArgsV2(req)
}
