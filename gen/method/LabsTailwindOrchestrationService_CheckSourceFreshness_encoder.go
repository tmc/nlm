package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeCheckSourceFreshnessArgs encodes arguments for LabsTailwindOrchestrationService.CheckSourceFreshness
// RPC ID: yR9Yof
//
// Wire format: [null, ["source-id"], [2]]
//   Field 2: SourceRevision sub-message with field 1 = source ID
//   Field 3: ProjectContext {field 1: 2}
func EncodeCheckSourceFreshnessArgs(req *notebooklmv1alpha1.CheckSourceFreshnessRequest) []interface{} {
	sourceRevision := []interface{}{req.GetSourceId()}
	projectContext := []interface{}{2}
	return []interface{}{nil, sourceRevision, projectContext}
}
