package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeRefreshSourceArgs encodes arguments for LabsTailwindOrchestrationService.RefreshSource.
// RPC ID: FLmJqe
//
// Wire format (HAR-verified 2026-04-17 against Google-Drive source
// 00000000-0000-4000-8000-000000000109 in notebook
// 00000000-0000-4000-8000-000000000006):
//
//	[null, ["source-id"], [2]]
//
// Shape is identical to CheckSourceFreshness (yR9Yof); both RPCs are
// Google-Drive-only and use ProjectContext value 2. The CLI layer is
// expected to reject non-Drive sources before dispatch; the server
// responds with "One or more arguments are invalid" for non-Drive
// source ids against this shape.
//
// Response shape is a rich Drive-metadata tuple — see
// internal/notebooklm/api/source_freshness.go parseRefreshSourceResponse.
func EncodeRefreshSourceArgs(req *notebooklmv1alpha1.RefreshSourceRequest) []interface{} {
	sourceRevision := []interface{}{req.GetSourceId()}
	projectContext := []interface{}{2}
	return []interface{}{nil, sourceRevision, projectContext}
}
