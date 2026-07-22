package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/rpc/argbuilder"
)

// GENERATION_BEHAVIOR: append

// EncodeListRecentlyViewedProjectsArgs encodes arguments for LabsTailwindOrchestrationService.ListRecentlyViewedProjects
// RPC ID: wXbhsf
// Argument format: [null, 1, null, [2, null, null, [1, null, null, null, null, null, null, null, null, null, [1]]], null, [[null, null, []], [[]], [null, []]]]
func EncodeListRecentlyViewedProjectsArgs(req *notebooklmv1alpha1.ListRecentlyViewedProjectsRequest) []interface{} {
	// Using generalized argument encoder. printf %q emits a properly escaped Go
	// string literal so arg_formats containing quotes (e.g. "New Note") stay valid.
	args, err := argbuilder.EncodeRPCArgs(req, "[null, 1, null, [2, null, null, [1, null, null, null, null, null, null, null, null, null, [1]]], null, [[null, null, []], [[]], [null, []]]]")
	if err != nil {
		// Log error and return empty args as fallback
		// In production, this should be handled better
		return []interface{}{}
	}
	return args
}
