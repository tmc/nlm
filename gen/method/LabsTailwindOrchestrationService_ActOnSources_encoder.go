package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeActOnSourcesArgs encodes arguments for LabsTailwindOrchestrationService.ActOnSources
// RPC ID: yyryJe
// Updated format based on actual browser API calls (November 2025)
// Format: [[[[source_ids]]],null,null,null,null,[action,[[context]],extra],null,[2,null,[1]]]
func EncodeActOnSourcesArgs(req *notebooklmv1alpha1.ActOnSourcesRequest) []interface{} {
	// Build nested source IDs array
	// Browser format in f.req args: [[[source_id]]] (3 levels in position 0)
	// Because the whole args array adds 1 level, position 0 needs 3 levels
	var sourceIDsInner []interface{}
	for _, sid := range req.GetSourceIds() {
		sourceIDsInner = append(sourceIDsInner, sid)
	}
	// sourceIDsInner = [sid] - 1 level
	// Wrap 2 more times: [[[sid]]]
	sourceIDsNested := []interface{}{[]interface{}{sourceIDsInner}}

	// Build action info: [action, [["[CONTEXT]", ""]], ""]
	actionInfo := []interface{}{
		req.GetAction(),
		[]interface{}{[]interface{}{"[CONTEXT]", ""}},
		"",
	}

	// Build the full argument array
	args := []interface{}{
		sourceIDsNested, // Position 0: [[[[source_ids]]]]
		nil,             // Position 1
		nil,             // Position 2
		nil,             // Position 3
		nil,             // Position 4
		actionInfo,      // Position 5: [action, [[context]], extra]
		nil,             // Position 6
		[]interface{}{2, nil, []interface{}{1}}, // Position 7: metadata
	}

	return args
}
