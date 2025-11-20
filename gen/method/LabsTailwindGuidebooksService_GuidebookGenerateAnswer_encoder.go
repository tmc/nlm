package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/rpc/argbuilder"
)

// GENERATION_BEHAVIOR: append

// EncodeGuidebookGenerateAnswerArgs encodes arguments for LabsTailwindGuidebooksService.GuidebookGenerateAnswer
// RPC ID: itA0pc
// Argument format: [%guidebook_id%, %question%, %settings%]
func EncodeGuidebookGenerateAnswerArgs(req *notebooklmv1alpha1.GuidebookGenerateAnswerRequest) []interface{} {
	// Using generalized argument encoder
	args, err := argbuilder.EncodeRPCArgs(req, "[%guidebook_id%, %question%, %settings%]")
	if err != nil {
		// Log error and return empty args as fallback
		// In production, this should be handled better
		return []interface{}{}
	}
	return args
}
