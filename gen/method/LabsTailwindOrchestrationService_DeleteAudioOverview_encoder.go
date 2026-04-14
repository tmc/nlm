package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeDeleteAudioOverviewArgs encodes arguments for LabsTailwindOrchestrationService.DeleteAudioOverview
// RPC ID: sJDbic
// Wire format verified against HAR capture (hizoJc):
//   [["<project_id>"], [2], [2]]
func EncodeDeleteAudioOverviewArgs(req *notebooklmv1alpha1.DeleteAudioOverviewRequest) []interface{} {
	return EncodeDeleteAudioOverviewArgsV2(req)
}
