package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeGetArtifactArgs encodes arguments for LabsTailwindOrchestrationService.GetArtifact
// RPC ID: BnLyuf
// Argument format: [%artifact_id%]
func EncodeGetArtifactArgs(req *notebooklmv1alpha1.GetArtifactRequest) []interface{} {
	// Single artifact ID encoding
	return []interface{}{req.GetArtifactId()}
}
