package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/rpc/argbuilder"
)

// GENERATION_BEHAVIOR: append

// EncodeDiscoverSourcesArgs encodes arguments for LabsTailwindOrchestrationService.DiscoverSources
// RPC ID: qXyaNe
// Argument format: [%project_id%, %query%]
func EncodeDiscoverSourcesArgs(req *notebooklmv1alpha1.DiscoverSourcesRequest) []interface{} {
	// Using generalized argument encoder
	args, err := argbuilder.EncodeRPCArgs(req, "[%project_id%, %query%]")
	if err != nil {
		// Log error and return empty args as fallback
		// In production, this should be handled better
		return []interface{}{}
	}
	return args
}
