package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeCreateArtifactArgs encodes arguments for LabsTailwindOrchestrationService.CreateArtifact
// RPC ID: xpWGLf
// Argument format: [%context%, %project_id%, %artifact%]
func EncodeCreateArtifactArgs(req *notebooklmv1alpha1.CreateArtifactRequest) []interface{} {
	// CreateArtifact encoding
	return []interface{}{encodeContext(req.GetContext()), req.GetProjectId(), encodeArtifact(req.GetArtifact())}
}
