package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// EncodeGenerateReportSuggestionsArgs encodes arguments for
// LabsTailwindOrchestrationService.GenerateReportSuggestions.
// RPC ID: ciyUvf (replaces the dead GHsKob).
//
// Wire format verified against HAR capture — do not regenerate.
// Shape: [ProjectContext, "notebook-id", [["src1"], ["src2"], ...]].
func EncodeGenerateReportSuggestionsArgs(req *notebooklmv1alpha1.GenerateReportSuggestionsRequest) []interface{} {
	var sourceRefs []interface{}
	for _, id := range req.GetSourceIds() {
		sourceRefs = append(sourceRefs, []interface{}{id})
	}

	projectContext := []interface{}{
		2, nil, nil,
		[]interface{}{1, nil, nil, nil, nil, nil, nil, nil, nil, nil, []interface{}{1}},
		[]interface{}{[]interface{}{1, 4, 2, 3, 6, 5}},
	}

	return []interface{}{
		projectContext,
		req.GetProjectId(),
		sourceRefs,
	}
}
