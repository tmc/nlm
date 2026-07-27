package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/argbuilder"
)

// GENERATION_BEHAVIOR: append

// EncodePollDeepResearchArgs encodes arguments for LabsTailwindOrchestrationService.PollDeepResearch
// RPC ID: e3bVqc
// Argument format: [%context%, null, %job_handle%]
func EncodePollDeepResearchArgs(req *notebooklmv1alpha1.PollDeepResearchRequest) []interface{} {
	// Using generalized argument encoder. printf %q emits a properly escaped Go
	// string literal so arg_formats containing quotes (e.g. "New Note") stay valid.
	args, err := argbuilder.EncodeRPCArgs(req, "[%context%, null, %job_handle%]")
	if err != nil {
		// Log error and return empty args as fallback
		// In production, this should be handled better
		return []interface{}{}
	}
	return args
}
