package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/argbuilder"
)

// GENERATION_BEHAVIOR: append

// EncodeGetDeepResearchSessionsArgs encodes arguments for LabsTailwindOrchestrationService.GetDeepResearchSessions
// RPC ID: e3bVqc
// Argument format: [null, null, %project_id%]
func EncodeGetDeepResearchSessionsArgs(req *notebooklmv1alpha1.GetDeepResearchSessionsRequest) []interface{} {
	// Using generalized argument encoder. printf %q emits a properly escaped Go
	// string literal so arg_formats containing quotes (e.g. "New Note") stay valid.
	args, err := argbuilder.EncodeRPCArgs(req, "[null, null, %project_id%]")
	if err != nil {
		// Log error and return empty args as fallback
		// In production, this should be handled better
		return []interface{}{}
	}
	return args
}
