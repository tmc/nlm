package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/rpc/argbuilder"
)

// GENERATION_BEHAVIOR: append

// EncodeGetConversationsArgs encodes arguments for LabsTailwindOrchestrationService.GetConversations
// RPC ID: hPTbtc
// Argument format: [[], null, %project_id%, %limit%]
func EncodeGetConversationsArgs(req *notebooklmv1alpha1.GetConversationsRequest) []interface{} {
	// Using generalized argument encoder
	args, err := argbuilder.EncodeRPCArgs(req, "[[], null, %project_id%, %limit%]")
	if err != nil {
		// Log error and return empty args as fallback
		// In production, this should be handled better
		return []interface{}{}
	}
	return args
}
