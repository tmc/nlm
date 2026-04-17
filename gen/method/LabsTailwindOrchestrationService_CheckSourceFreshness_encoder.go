package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeCheckSourceFreshnessArgs encodes arguments for LabsTailwindOrchestrationService.CheckSourceFreshness.
// RPC ID: yR9Yof
//
// Wire format (restored from commit ab27baa, pre-argbuilder-regression):
//
//	[null, ["source-id"], [4]]
//
// pos 0: null (reserved)
// pos 1: SourceRevision — [source-id]
// pos 2: ProjectContext — [4] (refresh/freshness RPCs use context value 4, not 2)
//
// Live-verified 2026-04-17: confirmed working shape against server for source
// 4f6dd32d-9942-405f-b208-5f6fd5e4e704 in notebook 00000000-0000-4000-8000-000000000005.
// See docs/dev/phase1-verification.md §2 (originally "error 3 (InvalidInput)" with
// argbuilder stub `[%source_id%]`).
func EncodeCheckSourceFreshnessArgs(req *notebooklmv1alpha1.CheckSourceFreshnessRequest) []interface{} {
	sourceRevision := []interface{}{req.GetSourceId()}
	projectContext := []interface{}{4}
	return []interface{}{nil, sourceRevision, projectContext}
}
