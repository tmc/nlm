package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// EncodeGenerateNotebookGuideArgs encodes arguments for
// LabsTailwindOrchestrationService.GenerateNotebookGuide.
// RPC ID: VfAZjd
//
// Wire format: unverified — no HAR capture available. This encoder is a
// best-effort shape based on the proto field order and the Milestone 0
// observed guide-type enum values.
//
//	[%project_id%, %guide_type%]
//
// guide_type is emitted as its int32 enum value (GUIDE_TYPE_OUTLINE=1,
// GUIDE_TYPE_MIND_MAP=2); the previous argbuilder-backed format
// dropped it silently.
//
// TODO: capture HAR for both GUIDE_TYPE_OUTLINE and GUIDE_TYPE_MIND_MAP
// and update this encoder + its guard comment to "verified". Until then,
// callers that depend on guide_type semantics should prefer ActOnSources
// (which is how `nlm mindmap` currently operates).
func EncodeGenerateNotebookGuideArgs(req *notebooklmv1alpha1.GenerateNotebookGuideRequest) []interface{} {
	return []interface{}{
		req.GetProjectId(),
		int32(req.GetGuideType()),
	}
}
