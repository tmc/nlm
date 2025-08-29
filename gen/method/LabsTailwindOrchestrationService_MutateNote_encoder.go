package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeMutateNoteArgs encodes arguments for LabsTailwindOrchestrationService.MutateNote
// RPC ID: cYAfTb
// Argument format: [%note_id%, %title%, %content%]
func EncodeMutateNoteArgs(req *notebooklmv1alpha1.MutateNoteRequest) []interface{} {
	// MutateNote has updates field instead of direct title/content
	var title, content string
	if len(req.GetUpdates()) > 0 {
		title = req.GetUpdates()[0].GetTitle()
		content = req.GetUpdates()[0].GetContent()
	}
	return []interface{}{
		req.GetNoteId(),
		title,
		content,
	}
}
