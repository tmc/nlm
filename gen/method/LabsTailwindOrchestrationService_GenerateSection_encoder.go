package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// Wire format verified against HAR capture — do not regenerate.

// EncodeGenerateSectionArgs encodes arguments for LabsTailwindOrchestrationService.GenerateSection
// RPC ID: BeTrYd
//
// Wire format: [project_id, [2]]
func EncodeGenerateSectionArgs(req *notebooklmv1alpha1.GenerateSectionRequest) []interface{} {
	return []interface{}{
		req.GetProjectId(),
		[]interface{}{2},
	}
}
