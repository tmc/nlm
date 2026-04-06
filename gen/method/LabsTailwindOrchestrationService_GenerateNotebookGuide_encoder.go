package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeGenerateNotebookGuideArgs encodes arguments for LabsTailwindOrchestrationService.GenerateNotebookGuide
// RPC ID: VfAZjd
//
// Wire format (confirmed via HAR): [project_id, [2]]
func EncodeGenerateNotebookGuideArgs(req *notebooklmv1alpha1.GenerateNotebookGuideRequest) []interface{} {
	return []interface{}{
		req.GetProjectId(),
		[]interface{}{2},
	}
}
