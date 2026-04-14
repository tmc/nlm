package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeShareGuidebookArgs encodes arguments for LabsTailwindGuidebooksService.ShareGuidebook
// RPC ID: OTl0K
// Wire format verified against HAR capture (sqTeoe):
//   [[2, null, null, [1,...,[1]], [[1,4,2,3,6,5]]], null, 1]
func EncodeShareGuidebookArgs(req *notebooklmv1alpha1.ShareGuidebookRequest) []interface{} {
	return EncodeShareGuidebookArgsV2(req)
}
