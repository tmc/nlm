package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeGetConversationHistoryArgs encodes arguments for LabsTailwindOrchestrationService.GetConversationHistory
// RPC ID: khqZz
//
// Wire format: [projectId, conversationId]
//
// pos 0: project/notebook ID
// pos 1: conversation ID
func EncodeGetConversationHistoryArgs(req *notebooklmv1alpha1.GetConversationHistoryRequest) []interface{} {
	return []interface{}{
		req.GetProjectId(),       // pos 0: project ID
		req.GetConversationId(),  // pos 1: conversation ID
	}
}
