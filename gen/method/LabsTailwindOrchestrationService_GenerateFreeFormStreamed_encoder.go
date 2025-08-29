package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeGenerateFreeFormStreamedArgs encodes arguments for LabsTailwindOrchestrationService.GenerateFreeFormStreamed
// RPC ID: BD
// Argument format: [%project_id%, %prompt%]
func EncodeGenerateFreeFormStreamedArgs(req *notebooklmv1alpha1.GenerateFreeFormStreamedRequest) []interface{} {
	return []interface{}{
		req.GetProjectId(),
		req.GetPrompt(),
	}
}
