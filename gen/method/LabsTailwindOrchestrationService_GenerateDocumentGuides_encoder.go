package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeGenerateDocumentGuidesArgs encodes arguments for LabsTailwindOrchestrationService.GenerateDocumentGuides
// RPC ID: tr032e
// Argument format: [%project_id%]
func EncodeGenerateDocumentGuidesArgs(req *notebooklmv1alpha1.GenerateDocumentGuidesRequest) []interface{} {
	// Single project ID encoding
	return []interface{}{req.GetProjectId()}
}
