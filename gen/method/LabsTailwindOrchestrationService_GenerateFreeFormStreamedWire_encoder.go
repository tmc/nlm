package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/rpc/argbuilder"
)

// GENERATION_BEHAVIOR: append

// EncodeGenerateFreeFormStreamedWireArgs encodes arguments for LabsTailwindOrchestrationService.GenerateFreeFormStreamedWire
// RPC ID: laWbsf
// Argument format: [%sources%, %prompt%, %history%, %options%, %conversation_id%, null, null, %notebook_id%, %sequence_number%]
func EncodeGenerateFreeFormStreamedWireArgs(req *notebooklmv1alpha1.GenerateFreeFormStreamedWireRequest) []interface{} {
	// Using generalized argument encoder. printf %q emits a properly escaped Go
	// string literal so arg_formats containing quotes (e.g. "New Note") stay valid.
	args, err := argbuilder.EncodeRPCArgs(req, "[%sources%, %prompt%, %history%, %options%, %conversation_id%, null, null, %notebook_id%, %sequence_number%]")
	if err != nil {
		// Log error and return empty args as fallback
		// In production, this should be handled better
		return []interface{}{}
	}
	return args
}
