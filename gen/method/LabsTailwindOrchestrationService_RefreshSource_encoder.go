package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeRefreshSourceArgs encodes arguments for LabsTailwindOrchestrationService.RefreshSource
// RPC ID: FLmJqe
//
// Wire format (inferred, not HAR-verified):
//
//	[null, ["source-id"], [2]]
//
// pos 0: null (reserved)
// pos 1: SourceRevision — [source-id]
// pos 2: ProjectContext — [2]
func EncodeRefreshSourceArgs(req *notebooklmv1alpha1.RefreshSourceRequest) []interface{} {
	sourceRevision := []interface{}{req.GetSourceId()}
	projectContext := []interface{}{2}
	return []interface{}{nil, sourceRevision, projectContext}
}
