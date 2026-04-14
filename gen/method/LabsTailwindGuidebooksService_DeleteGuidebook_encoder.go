package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeDeleteGuidebookArgs encodes arguments for LabsTailwindGuidebooksService.DeleteGuidebook
// RPC ID: ARGkVc
// Wire format verified against HAR capture (ozz5Z):
//   [[[[null, "<id>", <int>], [null,...,[null,null,2]], 1]]]
func EncodeDeleteGuidebookArgs(req *notebooklmv1alpha1.DeleteGuidebookRequest) []interface{} {
	return EncodeDeleteGuidebookArgsV2(req)
}
