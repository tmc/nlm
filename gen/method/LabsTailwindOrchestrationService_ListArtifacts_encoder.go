package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeListArtifactsArgs encodes arguments for LabsTailwindOrchestrationService.ListArtifacts
// RPC ID: LfTXoe
// Argument format: [%project_id%, %page_size%, %page_token%]
func EncodeListArtifactsArgs(req *notebooklmv1alpha1.ListArtifactsRequest) []interface{} {
	// ListArtifacts encoding
	return []interface{}{req.GetProjectId(), req.GetPageSize(), req.GetPageToken()}
}
