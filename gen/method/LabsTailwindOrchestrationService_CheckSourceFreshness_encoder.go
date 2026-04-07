package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeCheckSourceFreshnessArgs encodes arguments for LabsTailwindOrchestrationService.CheckSourceFreshness
// RPC ID: yR9Yof
//
// Wire format (from NLM source analysis):
//
//	[null, ["source-id"], [4]]
//
// pos 2: ProjectContext [4] — refresh/freshness RPCs use context value 4
func EncodeCheckSourceFreshnessArgs(req *notebooklmv1alpha1.CheckSourceFreshnessRequest) []interface{} {
	sourceRevision := []interface{}{req.GetSourceId()}
	projectContext := []interface{}{4}
	return []interface{}{nil, sourceRevision, projectContext}
}
