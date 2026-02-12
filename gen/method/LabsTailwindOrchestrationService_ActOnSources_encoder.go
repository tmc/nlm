package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeActOnSourcesArgs encodes arguments for LabsTailwindOrchestrationService.ActOnSources
// RPC ID: yyryJe
//
// Wire format for named actions (czb pattern — rephrase, expand, summarize, critique, etc.):
//   [sourceRefs, null, null, null, null, [actionName, []], null, [2, null, 1, [1]]]
//
// Field 1: repeated WB source references, each wrapping Ru{sourceId}
// Field 6: vZa named action {field 1: action name, field 2: [context objects]}
// Field 8: ProjectContext yZa variant {field 1: 2, field 3: 1, field 4: ZB{field 1: 1}}
func EncodeActOnSourcesArgs(req *notebooklmv1alpha1.ActOnSourcesRequest) []interface{} {
	// Build source references: each source ID wraps as WB{Ru{sourceId}} = [["sourceId"]]
	var sourceRefs []interface{}
	for _, id := range req.GetSourceIds() {
		sourceRefs = append(sourceRefs, []interface{}{[]interface{}{id}})
	}

	// Named action: vZa{field 1: action name, field 2: []}
	namedAction := []interface{}{req.GetAction(), []interface{}{}}

	// ProjectContext yZa variant: [2, null, 1, [1]]
	projectContext := []interface{}{2, nil, 1, []interface{}{1}}

	return []interface{}{
		sourceRefs,    // field 1: source refs
		nil,           // field 2: hZa context settings (null for named actions)
		nil,           // field 3: oneof YB (standard prompt)
		nil,           // field 4: oneof kZa (update chat)
		nil,           // field 5: gap
		namedAction,   // field 6: oneof vZa (named action)
		nil,           // field 7: aZa generation options
		projectContext, // field 8: ProjectContext
	}
}
