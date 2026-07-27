package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/rpc/argbuilder"
)

// GENERATION_BEHAVIOR: append

// EncodeBulkImportFromResearchWireArgs encodes arguments for LabsTailwindOrchestrationService.BulkImportFromResearchWire
// RPC ID: LBwxtb
// Argument format: [%context%, %marker%, %conversation_id%, %project_id%, %sources%]
func EncodeBulkImportFromResearchWireArgs(req *notebooklmv1alpha1.BulkImportFromResearchWireRequest) []interface{} {
	// Using generalized argument encoder. printf %q emits a properly escaped Go
	// string literal so arg_formats containing quotes (e.g. "New Note") stay valid.
	args, err := argbuilder.EncodeRPCArgs(req, "[%context%, %marker%, %conversation_id%, %project_id%, %sources%]")
	if err != nil {
		// Log error and return empty args as fallback
		// In production, this should be handled better
		return []interface{}{}
	}
	return args
}
