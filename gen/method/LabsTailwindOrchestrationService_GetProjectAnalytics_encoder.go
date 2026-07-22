package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/rpc/argbuilder"
)

// GENERATION_BEHAVIOR: append

// EncodeGetProjectAnalyticsArgs encodes arguments for LabsTailwindOrchestrationService.GetProjectAnalytics
// RPC ID: AUrzMb
// Argument format: [%project_id%, [2, null, null, [1, null, null, null, null, null, null, null, null, null, [1]]]]
func EncodeGetProjectAnalyticsArgs(req *notebooklmv1alpha1.GetProjectAnalyticsRequest) []interface{} {
	// Using generalized argument encoder. printf %q emits a properly escaped Go
	// string literal so arg_formats containing quotes (e.g. "New Note") stay valid.
	args, err := argbuilder.EncodeRPCArgs(req, "[%project_id%, [2, null, null, [1, null, null, null, null, null, null, null, null, null, [1]]]]")
	if err != nil {
		// Log error and return empty args as fallback
		// In production, this should be handled better
		return []interface{}{}
	}
	return args
}
