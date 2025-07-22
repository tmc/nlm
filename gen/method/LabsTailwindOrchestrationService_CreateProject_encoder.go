package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeCreateProjectArgs encodes arguments for LabsTailwindOrchestrationService.CreateProject
// RPC ID: CCqFvf
// Argument format: [%title%, %emoji%]
func EncodeCreateProjectArgs(req *notebooklmv1alpha1.CreateProjectRequest) []interface{} {
	// CreateProject encoding
	return []interface{}{req.GetTitle(), req.GetEmoji()}
}
