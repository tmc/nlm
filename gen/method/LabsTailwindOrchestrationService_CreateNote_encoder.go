package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeCreateNoteArgs encodes arguments for LabsTailwindOrchestrationService.CreateNote
// RPC ID: CYK0Xb
//
// Wire format: ["notebookId", "content", [noteType, null, null, displayFormat, sourceRefs], null, "title", null, [2]]
//   Field 1: string — notebook/project ID
//   Field 2: string — note content (body text)
//   Field 3: Wv metadata submessage {field 1: type, field 4: display format, field 5: source refs}
//   Field 5: string — note title
//   Field 7: ProjectContext [2]
func EncodeCreateNoteArgs(req *notebooklmv1alpha1.CreateNoteRequest) []interface{} {
	// Build source references for metadata (Ru sub-messages)
	var sourceRefs []interface{}
	for _, t := range req.GetNoteType() {
		// note_type field is repurposed to carry source type info
		_ = t
	}

	// Wv metadata: [noteType, null, null, displayFormat, sourceRefs]
	// For a basic note creation, type=0 (unspecified), displayFormat=0
	noteType := 0
	if len(req.GetNoteType()) > 0 {
		noteType = int(req.GetNoteType()[0])
	}
	metadata := []interface{}{noteType, nil, nil, 0, sourceRefs}

	// ProjectContext
	projectContext := []interface{}{2}

	return []interface{}{
		req.GetProjectId(), // field 1: notebook ID
		req.GetContent(),   // field 2: note content
		metadata,           // field 3: Wv metadata
		nil,                // field 4: repeated citations (optional)
		req.GetTitle(),     // field 5: note title
		nil,                // field 6: ov body reference (optional)
		projectContext,     // field 7: ProjectContext
	}
}
