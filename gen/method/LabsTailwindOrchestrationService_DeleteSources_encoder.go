package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeDeleteSourcesArgs encodes arguments for LabsTailwindOrchestrationService.DeleteSources
// RPC ID: tGMBJ
//
// Wire format from JS reverse-engineering: [repeated_source_ids, project_context]
//   - Field 1: repeated SourceId — each ID wrapped as ["id"]
//   - Field 2: ProjectContext [2]
func EncodeDeleteSourcesArgs(req *notebooklmv1alpha1.DeleteSourcesRequest) []interface{} {
	// Wire format verified against HAR capture — do not regenerate.
	wrappedIDs := make([]interface{}, len(req.GetSourceIds()))
	for i, id := range req.GetSourceIds() {
		wrappedIDs[i] = []interface{}{id}
	}
	return []interface{}{
		wrappedIDs,
		[]interface{}{2}, // ProjectContext
	}
}
