package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/rpc/argbuilder"
)

// GENERATION_BEHAVIOR: append

// EncodeActOnSourcesArgs encodes arguments for LabsTailwindOrchestrationService.ActOnSources
// RPC ID: yyryJe
// Argument format: [%project_id%, %action%, %source_ids%]
func EncodeActOnSourcesArgs(req *notebooklmv1alpha1.ActOnSourcesRequest) []interface{} {
	// Using generalized argument encoder
	args, err := argbuilder.EncodeRPCArgs(req, "[%project_id%, %action%, %source_ids%]")
	if err != nil {
		// Log error and return empty args as fallback
		// In production, this should be handled better
		return []interface{}{}
	}
	return args
}
