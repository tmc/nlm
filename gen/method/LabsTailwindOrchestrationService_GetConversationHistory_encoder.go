package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/rpc/argbuilder"
)

// GENERATION_BEHAVIOR: append

// EncodeGetConversationHistoryArgs encodes arguments for LabsTailwindOrchestrationService.GetConversationHistory
// RPC ID: khqZz
// Argument format: [[], null, null, %conversation_id%, %limit%]
func EncodeGetConversationHistoryArgs(req *notebooklmv1alpha1.GetConversationHistoryRequest) []interface{} {
	// Using generalized argument encoder
	args, err := argbuilder.EncodeRPCArgs(req, "[[], null, null, %conversation_id%, %limit%]")
	if err != nil {
		// Log error and return empty args as fallback
		// In production, this should be handled better
		return []interface{}{}
	}
	return args
}
