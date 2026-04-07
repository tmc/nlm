package method

import (
	"fmt"

	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeMutateNoteArgs encodes arguments for LabsTailwindOrchestrationService.MutateNote
// RPC ID: cYAfTb
//
// HAR-verified wire format:
//
//	[projectId, noteId, [[[htmlContent, title, [], 0]]], [2]]
//
// pos 0: project/notebook ID
// pos 1: note ID
// pos 2: updates array — triple-nested with [htmlContent, title, emptyArray, 0]
// pos 3: ProjectContext [2]
//
// Content must be HTML (e.g. "<p>text here</p>"). Plain text is wrapped in <p> tags.
func EncodeMutateNoteArgs(req *notebooklmv1alpha1.MutateNoteRequest) []interface{} {
	// Extract content and title from the first update
	var content, title string
	if len(req.GetUpdates()) > 0 {
		u := req.GetUpdates()[0]
		content = u.GetContent()
		title = u.GetTitle()
	}

	// Wrap plain text in <p> tags if not already HTML
	if content != "" && content[0] != '<' {
		content = fmt.Sprintf("<p>%s</p>", content)
	}

	// Build the updates structure: [[[htmlContent, title, [], 0]]]
	update := []interface{}{content, title, []interface{}{}, 0}
	updates := []interface{}{[]interface{}{update}}

	return []interface{}{
		req.GetProjectId(), // pos 0: project ID
		req.GetNoteId(),    // pos 1: note ID
		updates,            // pos 2: [[[content, title, [], 0]]]
		[]interface{}{2},   // pos 3: ProjectContext
	}
}
