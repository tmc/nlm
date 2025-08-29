package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeGenerateSectionArgs encodes arguments for LabsTailwindOrchestrationService.GenerateSection
// RPC ID: BeTrYd
// Argument format: [%project_id%]
func EncodeGenerateSectionArgs(req *notebooklmv1alpha1.GenerateSectionRequest) []interface{} {
	// Single project ID encoding
	return []interface{}{req.GetProjectId()}
}
