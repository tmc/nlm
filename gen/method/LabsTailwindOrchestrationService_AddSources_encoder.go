package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/rpc/argbuilder"
)

// GENERATION_BEHAVIOR: append

// EncodeAddSourcesArgs encodes arguments for LabsTailwindOrchestrationService.AddSources
// RPC ID: izAoDd
// Argument format: [%sources%, %project_id%]
func EncodeAddSourcesArgs(req *notebooklmv1alpha1.AddSourceRequest) []interface{} {
	// Using generalized argument encoder
	args, err := argbuilder.EncodeRPCArgs(req, "[%sources%, %project_id%]")
	if err != nil {
		// Log error and return empty args as fallback
		// In production, this should be handled better
		return []interface{}{}
	}
	return args
}
