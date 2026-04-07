package method

import (
	"fmt"

	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeCreateNoteArgs encodes arguments for LabsTailwindOrchestrationService.CreateNote
// RPC ID: CYK0Xb
//
// HAR-verified wire format:
//
//	[projectId, htmlContent, [1], null, title, null, [2]]
//
// pos 0: project/notebook ID
// pos 1: HTML content (empty string for new note, or "<p>text</p>" for content)
// pos 2: [1] (note type — user-created)
// pos 3: null
// pos 4: title string
// pos 5: null
// pos 6: ProjectContext [2]
//
// Content must be HTML. Plain text is wrapped in <p> tags.
func EncodeCreateNoteArgs(req *notebooklmv1alpha1.CreateNoteRequest) []interface{} {
	content := req.GetContent()
	if content != "" && content[0] != '<' {
		content = fmt.Sprintf("<p>%s</p>", content)
	}

	return []interface{}{
		req.GetProjectId(), // pos 0: project ID
		content,            // pos 1: HTML content
		[]interface{}{1},   // pos 2: [1] note type (user-created)
		nil,                // pos 3: null
		req.GetTitle(),     // pos 4: title
		nil,                // pos 5: null
		[]interface{}{2},   // pos 6: ProjectContext
	}
}
