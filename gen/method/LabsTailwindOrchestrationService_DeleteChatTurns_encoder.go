package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/argbuilder"
)

// GENERATION_BEHAVIOR: append

// EncodeDeleteChatTurnsArgs encodes arguments for LabsTailwindOrchestrationService.DeleteChatTurns
// RPC ID: J7Gthc
// Argument format: [%options%, %conversation_id%, null, %unknown_4%]
func EncodeDeleteChatTurnsArgs(req *notebooklmv1alpha1.DeleteChatTurnsRequest) []interface{} {
	// Using generalized argument encoder. printf %q emits a properly escaped Go
	// string literal so arg_formats containing quotes (e.g. "New Note") stay valid.
	args, err := argbuilder.EncodeRPCArgs(req, "[%options%, %conversation_id%, null, %unknown_4%]")
	if err != nil {
		// Log error and return empty args as fallback
		// In production, this should be handled better
		return []interface{}{}
	}
	return args
}
