package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/rpc/argbuilder"
)

// GENERATION_BEHAVIOR: append

// EncodeGetConversationHistoryArgs encodes arguments for LabsTailwindOrchestrationService.GetConversationHistory
// RPC ID: khqZz
// Argument format: [%project_id%, %conversation_id%]
func EncodeGetConversationHistoryArgs(req *notebooklmv1alpha1.GetConversationHistoryRequest) []interface{} {
	// Using generalized argument encoder
	args, err := argbuilder.EncodeRPCArgs(req, "[%project_id%, %conversation_id%]")
	if err != nil {
		// Log error and return empty args as fallback
		// In production, this should be handled better
		return []interface{}{}
	}
	return args
}
