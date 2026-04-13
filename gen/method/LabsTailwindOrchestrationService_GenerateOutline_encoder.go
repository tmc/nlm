package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// Wire format verified against HAR capture — do not regenerate.

// EncodeGenerateOutlineArgs encodes arguments for LabsTailwindOrchestrationService.GenerateOutline
// RPC ID: lCjAd
//
// Wire format: [project_id, [2]]
func EncodeGenerateOutlineArgs(req *notebooklmv1alpha1.GenerateOutlineRequest) []interface{} {
	return []interface{}{
		req.GetProjectId(),
		[]interface{}{2},
	}
}
