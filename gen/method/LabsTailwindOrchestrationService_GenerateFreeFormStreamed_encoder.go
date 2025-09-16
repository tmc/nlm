package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/rpc/argbuilder"
)

// GENERATION_BEHAVIOR: append

// EncodeGenerateFreeFormStreamedArgs encodes arguments for LabsTailwindOrchestrationService.GenerateFreeFormStreamed
// RPC ID: BD
// Argument format: [[%all_sources%], %prompt%, null, [2]] when sources present
// Fallback format: [%project_id%, %prompt%] when no sources
func EncodeGenerateFreeFormStreamedArgs(req *notebooklmv1alpha1.GenerateFreeFormStreamedRequest) []interface{} {
	// If sources are provided, use the gRPC format with sources
	if len(req.SourceIds) > 0 {
		// Build source array
		sourceArray := make([]interface{}, len(req.SourceIds))
		for i, sourceId := range req.SourceIds {
			sourceArray[i] = []interface{}{sourceId}
		}

		// Use gRPC format: [[%all_sources%], %prompt%, null, [2]]
		return []interface{}{
			[]interface{}{sourceArray},
			req.Prompt,
			nil,
			[]interface{}{2},
		}
	}

	// Fallback to old format without sources
	args, err := argbuilder.EncodeRPCArgs(req, "[%project_id%, %prompt%]")
	if err != nil {
		// Log error and return empty args as fallback
		// In production, this should be handled better
		return []interface{}{}
	}
	return args
}
