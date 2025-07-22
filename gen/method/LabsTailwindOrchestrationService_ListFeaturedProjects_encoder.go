package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeListFeaturedProjectsArgs encodes arguments for LabsTailwindOrchestrationService.ListFeaturedProjects
// RPC ID: nS9Qlc
// Argument format: [%page_size%, %page_token%]
func EncodeListFeaturedProjectsArgs(req *notebooklmv1alpha1.ListFeaturedProjectsRequest) []interface{} {
	// Pagination encoding
	return []interface{}{req.GetPageSize(), req.GetPageToken()}
}
