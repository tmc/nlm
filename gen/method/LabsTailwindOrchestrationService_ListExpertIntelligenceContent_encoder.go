package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/argbuilder"
)

// GENERATION_BEHAVIOR: append

// EncodeListExpertIntelligenceContentArgs encodes arguments for LabsTailwindOrchestrationService.ListExpertIntelligenceContent
// RPC ID: mVtEUb
// Argument format: [%context%, %content_kind%]
func EncodeListExpertIntelligenceContentArgs(req *notebooklmv1alpha1.ListExpertIntelligenceContentRequest) []interface{} {
	// Using generalized argument encoder. printf %q emits a properly escaped Go
	// string literal so arg_formats containing quotes (e.g. "New Note") stay valid.
	args, err := argbuilder.EncodeRPCArgs(req, "[%context%, %content_kind%]")
	if err != nil {
		// Log error and return empty args as fallback
		// In production, this should be handled better
		return []interface{}{}
	}
	return args
}
