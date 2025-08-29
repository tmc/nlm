package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeSubmitFeedbackArgs encodes arguments for LabsTailwindOrchestrationService.SubmitFeedback
// RPC ID: uNyJKe
// Argument format: [%project_id%, %feedback_type%, %feedback_text%]
func EncodeSubmitFeedbackArgs(req *notebooklmv1alpha1.SubmitFeedbackRequest) []interface{} {
	return []interface{}{
		req.GetProjectId(),
		req.GetFeedbackType(),
		req.GetFeedbackText(),
	}
}
