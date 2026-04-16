package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/rpc/argbuilder"
)

// GENERATION_BEHAVIOR: append

// EncodeMutateNoteArgs encodes arguments for LabsTailwindOrchestrationService.MutateNote
// RPC ID: cYAfTb
// Argument format: [%note_id%, %title%, %content%]
func EncodeMutateNoteArgs(req *notebooklmv1alpha1.MutateNoteRequest) []interface{} {
	// Using generalized argument encoder
	args, err := argbuilder.EncodeRPCArgs(req, "[%note_id%, %title%, %content%]")
	if err != nil {
		// Log error and return empty args as fallback
		// In production, this should be handled better
		return []interface{}{}
	}
	return args
}
