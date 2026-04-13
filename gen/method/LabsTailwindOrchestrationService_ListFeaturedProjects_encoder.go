package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/rpc/argbuilder"
)

// GENERATION_BEHAVIOR: append

// EncodeListFeaturedProjectsArgs encodes arguments for LabsTailwindOrchestrationService.ListFeaturedProjects
// RPC ID: ub2Bae
// Argument format: [[2]]
func EncodeListFeaturedProjectsArgs(req *notebooklmv1alpha1.ListFeaturedProjectsRequest) []interface{} {
	// Using generalized argument encoder
	args, err := argbuilder.EncodeRPCArgs(req, "[[2]]")
	if err != nil {
		// Log error and return empty args as fallback
		// In production, this should be handled better
		return []interface{}{}
	}
	return args
}
