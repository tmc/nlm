package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeRefreshSourceArgs encodes arguments for LabsTailwindOrchestrationService.RefreshSource
// RPC ID: FLmJqe
//
// Wire format (from NLM source analysis — CheckSourceFreshness uses same structure):
//
//	[null, ["source-id"], [4]]
//
// pos 0: null (reserved)
// pos 1: SourceRevision — [source-id]
// pos 2: ProjectContext — [4] (not [2]; refresh/freshness RPCs use context value 4)
func EncodeRefreshSourceArgs(req *notebooklmv1alpha1.RefreshSourceRequest) []interface{} {
	sourceRevision := []interface{}{req.GetSourceId()}
	projectContext := []interface{}{4}
	return []interface{}{nil, sourceRevision, projectContext}
}
