package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeStartSectionArgs encodes arguments for LabsTailwindOrchestrationService.StartSection
// RPC ID: pGC7gf
// Argument format: [%project_id%]
func EncodeStartSectionArgs(req *notebooklmv1alpha1.StartSectionRequest) []interface{} {
	// Single project ID encoding
	return []interface{}{req.GetProjectId()}
}
