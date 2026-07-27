package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/rpc/argbuilder"
)

// GENERATION_BEHAVIOR: append

// EncodeGenerateReportSuggestionsArgs encodes arguments for LabsTailwindOrchestrationService.GenerateReportSuggestions
// RPC ID: ciyUvf
// Argument format: [%context%, %project_id%, %source_ids%]
func EncodeGenerateReportSuggestionsArgs(req *notebooklmv1alpha1.GenerateReportSuggestionsRequest) []interface{} {
	// Using generalized argument encoder. printf %q emits a properly escaped Go
	// string literal so arg_formats containing quotes (e.g. "New Note") stay valid.
	args, err := argbuilder.EncodeRPCArgs(req, "[%context%, %project_id%, %source_ids%]")
	if err != nil {
		// Log error and return empty args as fallback
		// In production, this should be handled better
		return []interface{}{}
	}
	return args
}
