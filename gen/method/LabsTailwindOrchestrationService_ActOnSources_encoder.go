package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeActOnSourcesArgs encodes arguments for LabsTailwindOrchestrationService.ActOnSources
// RPC ID: yyryJe
//
// Wire format verified against HAR capture — do not regenerate.
//
// Wire format from HAR:
//
//	[sourceRefs, null, null, null, null, actionConfig, null, modeSelector]
//
// sourceRefs: [[["source-id-1"]], [["source-id-2"]]]  (each source: [["id"]], 2-level)
// actionConfig: ["action_name", [["[CONTEXT]", ""]], ""]  (3-element array)
// modeSelector: [2, null, [1], [1]]  (same as chat mode selector)
// Notebook ID is conveyed via source-path URL param, NOT in args.
func EncodeActOnSourcesArgs(req *notebooklmv1alpha1.ActOnSourcesRequest) []interface{} {
	// Wire format verified against HAR capture — do not regenerate.

	// Build source references: each source is [["uuid"]] (2-level nesting, same as chat)
	var sourceRefs []interface{}
	for _, id := range req.GetSourceIds() {
		sourceRefs = append(sourceRefs, []interface{}{[]interface{}{id}})
	}

	// Action config: [action_name, [["[CONTEXT]", ""]], ""]
	actionConfig := []interface{}{
		req.GetAction(),
		[]interface{}{[]interface{}{"[CONTEXT]", ""}},
		"",
	}

	// Mode selector (same as chat): [2, null, [1], [1]]
	modeSelector := []interface{}{2, nil, []interface{}{1}, []interface{}{1}}

	return []interface{}{
		sourceRefs,   // pos 0: source refs
		nil,          // pos 1
		nil,          // pos 2
		nil,          // pos 3
		nil,          // pos 4
		actionConfig, // pos 5: action config
		nil,          // pos 6
		modeSelector, // pos 7: mode selector
	}
}
