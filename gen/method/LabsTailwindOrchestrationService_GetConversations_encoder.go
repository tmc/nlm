package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// Wire format verified against HAR capture — do not regenerate.

// EncodeGetConversationsArgs encodes arguments for LabsTailwindOrchestrationService.GetConversations
// RPC ID: hPTbtc
//
// Wire format: [[], null, projectId, limit]
//
// pos 0: empty array
// pos 1: null
// pos 2: project/notebook ID
// pos 3: query limit (number of recent conversations to fetch)
func EncodeGetConversationsArgs(req *notebooklmv1alpha1.GetConversationsRequest) []interface{} {
	limit := req.GetLimit()
	if limit == 0 {
		limit = 20 // default to 20 most recent
	}
	return []interface{}{
		[]interface{}{},    // pos 0: empty array
		nil,                // pos 1: null
		req.GetProjectId(), // pos 2: project ID
		limit,              // pos 3: limit
	}
}
