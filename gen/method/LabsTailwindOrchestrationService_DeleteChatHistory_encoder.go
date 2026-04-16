package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/rpc/argbuilder"
)

// GENERATION_BEHAVIOR: append

// EncodeDeleteChatHistoryArgs encodes arguments for LabsTailwindOrchestrationService.DeleteChatHistory
// RPC ID: e3bVqc
// Argument format: [null, null, %project_id%]
func EncodeDeleteChatHistoryArgs(req *notebooklmv1alpha1.DeleteChatHistoryRequest) []interface{} {
	// Using generalized argument encoder
	args, err := argbuilder.EncodeRPCArgs(req, "[null, null, %project_id%]")
	if err != nil {
		// Log error and return empty args as fallback
		// In production, this should be handled better
		return []interface{}{}
	}
	return args
}
