package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeCheckSourceFreshnessArgs encodes arguments for LabsTailwindOrchestrationService.CheckSourceFreshness.
// RPC ID: yR9Yof
//
// Wire format (HAR-verified 2026-04-17 against Google-Drive source
// 00000000-0000-4000-8000-000000000109 in notebook
// 00000000-0000-4000-8000-000000000006):
//
//	[null, ["source-id"], [2]]
//
// pos 0: null (reserved)
// pos 1: SourceRevision — [source-id]
// pos 2: ProjectContext — [2] (freshness context value)
//
// Retraction note: commit ab27baa documented the ProjectContext as [4],
// claiming it was "live-verified." That verification was conducted
// against non-Google-Drive sources where the server silently accepts
// the wrong shape and returns garbage. Against real Drive sources the
// [4] shape is rejected with "One or more arguments are invalid."
// The correct value is [2] as shown by HAR captures of the live UI.
// The client gates refresh to Drive sources to avoid that false-positive path.
func EncodeCheckSourceFreshnessArgs(req *notebooklmv1alpha1.CheckSourceFreshnessRequest) []interface{} {
	sourceRevision := []interface{}{req.GetSourceId()}
	projectContext := []interface{}{2}
	return []interface{}{nil, sourceRevision, projectContext}
}
