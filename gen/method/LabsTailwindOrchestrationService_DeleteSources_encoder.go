package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeDeleteSourcesArgs encodes arguments for LabsTailwindOrchestrationService.DeleteSources
// RPC ID: tGMBJ
//
// Wire format: [[["source-id-1"], ["source-id-2"]], [2]]
//   Field 1: repeated SourceRevision messages (each wraps source ID in sub-message)
//   Field 2: ProjectContext {field 1: 2}
func EncodeDeleteSourcesArgs(req *notebooklmv1alpha1.DeleteSourcesRequest) []interface{} {
	// Build repeated SourceRevision messages: [["id1"], ["id2"]]
	var sourceRevisions []interface{}
	for _, id := range req.GetSourceIds() {
		sourceRevisions = append(sourceRevisions, []interface{}{id})
	}
	// ProjectContext: [2]
	projectContext := []interface{}{2}
	return []interface{}{sourceRevisions, projectContext}
}
