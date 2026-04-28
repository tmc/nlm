package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeMutateNoteArgs encodes arguments for LabsTailwindOrchestrationService.MutateNote
// RPC ID: cYAfTb
//
// HAR-verified wire format (2026-04-28):
//
//	[project_id, note_id, [[[content, title, [tags...], 0]]], [2]]
//
// Note: content (body) precedes title in the inner update tuple. The triple
// wrap is intentional — the field is repeated NoteUpdate. Tags is an array of
// strings (empty slice in the typical UI flow). Trailing 0 is opaque.
func EncodeMutateNoteArgs(req *notebooklmv1alpha1.MutateNoteRequest) []interface{} {
	updates := req.GetUpdates()
	updateTuples := make([]interface{}, 0, len(updates))
	for _, u := range updates {
		tags := u.GetTags()
		tagsAny := make([]interface{}, len(tags))
		for i, t := range tags {
			tagsAny[i] = t
		}
		updateTuples = append(updateTuples, []interface{}{
			u.GetContent(),
			u.GetTitle(),
			tagsAny,
			0,
		})
	}
	return []interface{}{
		req.GetProjectId(),
		req.GetNoteId(),
		[]interface{}{updateTuples},
		[]interface{}{2},
	}
}
