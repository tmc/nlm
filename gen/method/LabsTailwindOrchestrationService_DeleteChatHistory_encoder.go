package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeDeleteChatHistoryArgs encodes arguments for LabsTailwindOrchestrationService.DeleteChatHistory
// RPC ID: e3bVqc
//
// Wire format: [null, null, projectId]
//
// Note: e3bVqc is a polymorphic endpoint. The payload shape determines the operation:
//   - [null, null, projectId] → DeleteChatHistory
//   - other shapes → PollDeepResearch
//
// pos 0: null
// pos 1: null
// pos 2: project/notebook ID
func EncodeDeleteChatHistoryArgs(req *notebooklmv1alpha1.DeleteChatHistoryRequest) []interface{} {
	return []interface{}{
		nil,                // pos 0: null
		nil,                // pos 1: null
		req.GetProjectId(), // pos 2: project ID
	}
}
