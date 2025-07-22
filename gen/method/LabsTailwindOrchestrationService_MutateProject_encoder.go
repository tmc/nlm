package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeMutateProjectArgs encodes arguments for LabsTailwindOrchestrationService.MutateProject
// RPC ID: s0tc2d
// Argument format: [%project_id%, %updates%]
func EncodeMutateProjectArgs(req *notebooklmv1alpha1.MutateProjectRequest) []interface{} {
	// MutateProject encoding
	return []interface{}{req.GetProjectId(), encodeProjectUpdates(req.GetUpdates())}
}
