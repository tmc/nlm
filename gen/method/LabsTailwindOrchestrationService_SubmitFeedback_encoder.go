package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeSubmitFeedbackArgs encodes arguments for LabsTailwindOrchestrationService.SubmitFeedback
// RPC ID: uNyJKe
// Wire format verified against HAR capture — do not regenerate.
//
// Wire format for general feedback (thumbs up/down on note/artifact):
//
//	[contextOneof, [rating, text, categories], textPair, audioCtx, ...]
//
// For simple text feedback submission:
//
//	field 2: R2a {field 1: rating (1=up, 4=down), field 2: text}
//	field 9: P2a {field 1: noteId, field 2: projectId} (for note context)
//
// Note: SubmitFeedback does NOT use a ProjectContext wrapper.
// The project ID is embedded in the context oneof messages.
func EncodeSubmitFeedbackArgs(req *notebooklmv1alpha1.SubmitFeedbackRequest) []interface{} {
	// For general feedback, use rating=1 (thumbs up) with text
	rating := 1
	feedbackDetails := []interface{}{rating, req.GetFeedbackText()}

	// For general feedback without a specific context, use project ID as note context
	noteContext := []interface{}{req.GetFeedbackType(), req.GetProjectId()}

	return []interface{}{
		nil,             // field 1: chat turn feedback context (oneof)
		feedbackDetails, // field 2: R2a feedback details
		nil,             // field 3: U2a text pair
		nil,             // field 4: audio feedback context (oneof)
		nil,             // field 5: gap
		nil,             // field 6: gap
		nil,             // field 7: gap
		nil,             // field 8: gap
		noteContext,     // field 9: P2a note/artifact feedback context (oneof)
	}
}
