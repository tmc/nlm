package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeListRecentlyViewedProjectsArgs encodes arguments for LabsTailwindOrchestrationService.ListRecentlyViewedProjects
// RPC ID: wXbhsf
// Argument format: [null, 1, null, [2]]
func EncodeListRecentlyViewedProjectsArgs(req *notebooklmv1alpha1.ListRecentlyViewedProjectsRequest) []interface{} {
	// Special case for ListRecentlyViewedProjects
	return []interface{}{nil, 1, nil, []int{2}}
}
