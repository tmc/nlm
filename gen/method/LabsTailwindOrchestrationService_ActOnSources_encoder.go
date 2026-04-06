package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeActOnSourcesArgs encodes arguments for LabsTailwindOrchestrationService.ActOnSources
// RPC ID: yyryJe
//
// Wire format from HAR:
//   [sourceRefs, null, null, null, null, action, null, [notebook_id, [2]]]
//
// sourceRefs: [[["source-id-1"]], [["source-id-2"]]]  (each source: [["id"]])
// action: string like "summarize", "brainstorm", etc.
// position 7: [notebook_id, [2]] (ProjectContext)
func EncodeActOnSourcesArgs(req *notebooklmv1alpha1.ActOnSourcesRequest) []interface{} {
	// Build source references: each source wraps as [["sourceId"]]
	var sourceRefs []interface{}
	for _, id := range req.GetSourceIds() {
		sourceRefs = append(sourceRefs, []interface{}{[]interface{}{id}})
	}

	return []interface{}{
		sourceRefs,                                          // pos 0: source refs
		nil,                                                 // pos 1
		nil,                                                 // pos 2
		nil,                                                 // pos 3
		nil,                                                 // pos 4
		req.GetAction(),                                     // pos 5: action string
		nil,                                                 // pos 6
		[]interface{}{req.GetProjectId(), []interface{}{2}},  // pos 7: [notebook_id, [2]]
	}
}
