package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeGuidebookGenerateAnswerArgs encodes arguments for LabsTailwindGuidebooksService.GuidebookGenerateAnswer
// RPC ID: itA0pc
// Argument format: [%guidebook_id%, %question%, %settings%]
func EncodeGuidebookGenerateAnswerArgs(req *notebooklmv1alpha1.GuidebookGenerateAnswerRequest) []interface{} {
	return []interface{}{
		req.GetGuidebookId(),
		req.GetQuestion(),
		req.GetSettings(),
	}
}
