package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeGuidebookGenerateAnswerArgs encodes arguments for LabsTailwindGuidebooksService.GuidebookGenerateAnswer
// RPC ID: itA0pc
// Wire format verified against HAR capture (eyWvXc):
//
//	["<question>", "<guidebook_id>", 0, "<notebook_id>"]
func EncodeGuidebookGenerateAnswerArgs(req *notebooklmv1alpha1.GuidebookGenerateAnswerRequest) []interface{} {
	return EncodeGuidebookGenerateAnswerArgsV2(req)
}
