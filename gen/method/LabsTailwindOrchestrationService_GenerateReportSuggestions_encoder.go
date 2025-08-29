package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeGenerateReportSuggestionsArgs encodes arguments for LabsTailwindOrchestrationService.GenerateReportSuggestions
// RPC ID: GHsKob
// Argument format: [%project_id%]
func EncodeGenerateReportSuggestionsArgs(req *notebooklmv1alpha1.GenerateReportSuggestionsRequest) []interface{} {
	// Single project ID encoding
	return []interface{}{req.GetProjectId()}
}
