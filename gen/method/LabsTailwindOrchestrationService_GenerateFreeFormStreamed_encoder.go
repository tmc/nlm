package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeGenerateFreeFormStreamedArgs encodes arguments for LabsTailwindOrchestrationService.GenerateFreeFormStreamed
// RPC ID: BD
// Updated format based on browser API calls pattern (November 2025)
// Format similar to ActOnSources: [[[source_ids]],prompt,null,null,null,null,null,[2,null,[1]]]
func EncodeGenerateFreeFormStreamedArgs(req *notebooklmv1alpha1.GenerateFreeFormStreamedRequest) []interface{} {
	// Build source IDs array with 2 levels of nesting
	// Final format in f.req: [[[source_id]]] (3 levels after JSON serialization adds 1)
	var sourceIDsInner []interface{}
	for _, sid := range req.GetSourceIds() {
		sourceIDsInner = append(sourceIDsInner, sid)
	}
	// Wrap 1 time: [[sid]]
	sourceIDsNested := []interface{}{sourceIDsInner}

	// Build the full argument array
	args := []interface{}{
		sourceIDsNested,  // Position 0: [[source_ids]]
		req.GetPrompt(),  // Position 1: prompt text
		nil,              // Position 2
		nil,              // Position 3
		nil,              // Position 4
		nil,              // Position 5
		nil,              // Position 6
		[]interface{}{2, nil, []interface{}{1}}, // Position 7: metadata
	}

	return args
}
