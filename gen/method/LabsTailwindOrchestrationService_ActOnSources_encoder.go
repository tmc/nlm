package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeActOnSourcesArgs encodes arguments for LabsTailwindOrchestrationService.ActOnSources
// RPC ID: yyryJe
//
// Uses the Vyb pattern (standard prompt action with source references):
//
// Wire format:
//   [sourceRefs, hZa, YB, null, null, null, null, ProjectContext, null, null, origin]
//
// Field 1: repeated WB source references wrapping Ru{sourceId}: [[[sourceId]]]
// Field 2: hZa context settings {field 7: 2, field 10: 2}
// Field 3: oneof YB standard action {field 1: prompt, field 2: [], field 4: actionType}
// Field 8: ProjectContext yZa variant [2, null, 1, [1]]
// Field 11: int origin enum
func EncodeActOnSourcesArgs(req *notebooklmv1alpha1.ActOnSourcesRequest) []interface{} {
	// Build source references: each source ID wraps as WB{oneof Ru{sourceId}} = [["sourceId"]]
	var sourceRefs []interface{}
	for _, id := range req.GetSourceIds() {
		sourceRefs = append(sourceRefs, []interface{}{[]interface{}{id}})
	}

	// hZa context settings: {field 7: 2, field 10: 2}
	// Wire: [null, null, null, null, null, null, 2, null, null, 2]
	hzaContext := []interface{}{nil, nil, nil, nil, nil, nil, 2, nil, nil, 2}

	// YB standard action: {field 1: prompt, field 2: annotations, field 4: action_type}
	// For named actions like "summarize", the prompt describes the action
	actionPrompt := "Please " + req.GetAction() + " the selected sources."
	yb := []interface{}{actionPrompt, []interface{}{}, nil, req.GetAction()}

	// ProjectContext yZa variant: [2, null, 1, [1]]
	projectContext := []interface{}{2, nil, 1, []interface{}{1}}

	return []interface{}{
		sourceRefs,     // field 1: source refs
		hzaContext,     // field 2: hZa context settings
		yb,             // field 3: oneof YB (standard prompt action)
		nil,            // field 4: oneof kZa (update chat)
		nil,            // field 5: gap
		nil,            // field 6: oneof vZa (named action) — not used in Vyb
		nil,            // field 7: aZa generation options
		projectContext, // field 8: ProjectContext
	}
}
