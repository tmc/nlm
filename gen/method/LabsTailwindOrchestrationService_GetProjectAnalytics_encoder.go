package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeGetProjectAnalyticsArgs encodes arguments for LabsTailwindOrchestrationService.GetProjectAnalytics
// RPC ID: AUrzMb
// Wire format verified against HAR capture (cFji9):
//
//	["<project_id>", null, [<timestamp_sec>, <timestamp_nanos>], [2]]
func EncodeGetProjectAnalyticsArgs(req *notebooklmv1alpha1.GetProjectAnalyticsRequest) []interface{} {
	return EncodeGetProjectAnalyticsArgsV2(req)
}
