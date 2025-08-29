package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeListRecentlyViewedGuidebooksArgs encodes arguments for LabsTailwindGuidebooksService.ListRecentlyViewedGuidebooks
// RPC ID: YJBpHc
// Argument format: [%page_size%, %page_token%]
func EncodeListRecentlyViewedGuidebooksArgs(req *notebooklmv1alpha1.ListRecentlyViewedGuidebooksRequest) []interface{} {
	// Pagination encoding
	return []interface{}{req.GetPageSize(), req.GetPageToken()}
}
