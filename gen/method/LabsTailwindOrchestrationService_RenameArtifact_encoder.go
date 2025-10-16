package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeRenameArtifactArgs encodes arguments for LabsTailwindOrchestrationService.RenameArtifact
// RPC ID: rc3d8d
// Argument format: [[artifactID, newTitle], [["title"]]]
// This is a manual implementation due to the complex nested structure with string literals
func EncodeRenameArtifactArgs(req *notebooklmv1alpha1.RenameArtifactRequest) []interface{} {
	return []interface{}{
		[]interface{}{req.ArtifactId, req.NewTitle},
		[]interface{}{[]interface{}{"title"}},
	}
}