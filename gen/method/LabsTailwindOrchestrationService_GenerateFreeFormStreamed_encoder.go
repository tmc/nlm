package method

import (
	"github.com/google/uuid"
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeGenerateFreeFormStreamedArgs encodes arguments for LabsTailwindOrchestrationService.GenerateFreeFormStreamed
// RPC ID: BD
//
// Captured format from browser (2026-02-01):
// [
//   [[[source_id_1]], [[source_id_2]]],  // [0] Sources - each wrapped as [[id]]
//   "prompt",                            // [1] User question
//   [],                                  // [2] Chat history (empty for first message)
//   [2, null, [1], [1]],                 // [3] Config options
//   "session_uuid",                      // [4] Chat session ID
//   null,                                // [5]
//   null,                                // [6]
//   "project_id",                        // [7] Notebook/project ID
//   1                                    // [8] Flag
// ]
func EncodeGenerateFreeFormStreamedArgs(req *notebooklmv1alpha1.GenerateFreeFormStreamedRequest) []interface{} {
	// Build source array where each source is wrapped as [[id]]
	sourceArray := make([]interface{}, len(req.SourceIds))
	for i, sourceId := range req.SourceIds {
		sourceArray[i] = []interface{}{[]interface{}{sourceId}}
	}

	// Generate a new session UUID for each chat request
	sessionID := uuid.New().String()

	// Chat history - empty for now (could be extended to support conversation context)
	chatHistory := []interface{}{}

	// Config options: [2, null, [1], [1]]
	config := []interface{}{2, nil, []interface{}{1}, []interface{}{1}}

	return []interface{}{
		sourceArray,      // [0] Sources as [[[id1]], [[id2]], ...]
		req.Prompt,       // [1] The prompt/question
		chatHistory,      // [2] Chat history
		config,           // [3] Config options
		sessionID,        // [4] Chat session UUID
		nil,              // [5] null
		nil,              // [6] null
		req.ProjectId,    // [7] Project/notebook ID
		1,                // [8] Flag
	}
}
