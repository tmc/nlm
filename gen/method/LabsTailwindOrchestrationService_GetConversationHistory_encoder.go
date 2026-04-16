package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeGetConversationHistoryArgs encodes arguments for LabsTailwindOrchestrationService.GetConversationHistory
// RPC ID: khqZz
// Wire format (HAR-verified): [[], null, null, conversation_id, limit]
// Project ID is conveyed via the source-path URL parameter, not in the body.
func EncodeGetConversationHistoryArgs(req *notebooklmv1alpha1.GetConversationHistoryRequest) []interface{} {
	return []interface{}{
		[]interface{}{},
		nil,
		nil,
		req.GetConversationId(),
		20,
	}
}
