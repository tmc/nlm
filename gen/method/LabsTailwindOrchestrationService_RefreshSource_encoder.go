package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeRefreshSourceArgs encodes arguments for LabsTailwindOrchestrationService.RefreshSource
// RPC ID: FLmJqe
// Wire format verified against HAR capture — do not regenerate.
//
// Wire format: [null, ["source-id"], [2]]
//   Field 2: SourceRevision sub-message with field 1 = source ID
//   Field 3: ProjectContext {field 1: 2}
func EncodeRefreshSourceArgs(req *notebooklmv1alpha1.RefreshSourceRequest) []interface{} {
	sourceRevision := []interface{}{req.GetSourceId()}
	projectContext := []interface{}{2}
	return []interface{}{nil, sourceRevision, projectContext}
}
