package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeDeleteArtifactArgs encodes arguments for LabsTailwindOrchestrationService.DeleteArtifact
// RPC ID: WxBZtb
// Wire format (inferred from ProjectContext pattern):
//
//	["<artifact_id>", [2]]
func EncodeDeleteArtifactArgs(req *notebooklmv1alpha1.DeleteArtifactRequest) []interface{} {
	return EncodeDeleteArtifactArgsV2(req)
}
