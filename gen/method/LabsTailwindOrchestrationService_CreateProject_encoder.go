package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/rpc/argbuilder"
)

// GENERATION_BEHAVIOR: append

// EncodeCreateProjectArgs encodes arguments for LabsTailwindOrchestrationService.CreateProject
// RPC ID: CCqFvf
// Argument format: [%title%, %emoji%]
func EncodeCreateProjectArgs(req *notebooklmv1alpha1.CreateProjectRequest) []interface{} {
	// Using generalized argument encoder
	args, err := argbuilder.EncodeRPCArgs(req, "[%title%, %emoji%]")
	if err != nil {
		// Log error and return empty args as fallback
		// In production, this should be handled better
		return []interface{}{}
	}
	return args
}
