package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/argbuilder"
)

// GENERATION_BEHAVIOR: append

// EncodeGenerateArtifactSuggestionsArgs encodes arguments for LabsTailwindOrchestrationService.GenerateArtifactSuggestions
// RPC ID: otmP3b
// Argument format: [[2, null, [1], [1, null, null, null, null, null, null, null, null, null, [1, 3]]], %project_id%, %source_refs%, %variation%, null, %prompt%]
func EncodeGenerateArtifactSuggestionsArgs(req *notebooklmv1alpha1.GenerateArtifactSuggestionsRequest) []interface{} {
	// Using generalized argument encoder. printf %q emits a properly escaped Go
	// string literal so arg_formats containing quotes (e.g. "New Note") stay valid.
	args, err := argbuilder.EncodeRPCArgs(req, "[[2, null, [1], [1, null, null, null, null, null, null, null, null, null, [1, 3]]], %project_id%, %source_refs%, %variation%, null, %prompt%]")
	if err != nil {
		// Log error and return empty args as fallback
		// In production, this should be handled better
		return []interface{}{}
	}
	return args
}
