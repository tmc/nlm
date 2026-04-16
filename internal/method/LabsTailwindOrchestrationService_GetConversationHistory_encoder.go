package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// EncodeGetConversationHistoryArgs encodes arguments for LabsTailwindOrchestrationService.GetConversationHistory.
// RPC ID: khqZz
//
// Wire format verified against HAR capture — do not regenerate.
// Shape: [[], null, null, conversation_id, limit]. The project ID travels in
// the source-path URL parameter, not in the argument body.
func EncodeGetConversationHistoryArgs(req *notebooklmv1alpha1.GetConversationHistoryRequest) []interface{} {
	return []interface{}{
		[]interface{}{},
		nil,
		nil,
		req.GetConversationId(),
		20,
	}
}
