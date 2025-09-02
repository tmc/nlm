package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/rpc/argbuilder"
)

// GENERATION_BEHAVIOR: append

// EncodeDeleteProjectsArgs encodes arguments for LabsTailwindOrchestrationService.DeleteProjects
// RPC ID: WWINqb
// Argument format: [%project_ids%]
func EncodeDeleteProjectsArgs(req *notebooklmv1alpha1.DeleteProjectsRequest) []interface{} {
	// Using generalized argument encoder
	args, err := argbuilder.EncodeRPCArgs(req, "[%project_ids%]")
	if err != nil {
		// Log error and return empty args as fallback
		// In production, this should be handled better
		return []interface{}{}
	}
	return args
}
