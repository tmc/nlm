package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/rpc/argbuilder"
)

// GENERATION_BEHAVIOR: append

// EncodeDeleteNotesArgs encodes arguments for LabsTailwindOrchestrationService.DeleteNotes
// RPC ID: AH0mwd
// Argument format: [%project_id%, null, %note_ids%, [2]]
func EncodeDeleteNotesArgs(req *notebooklmv1alpha1.DeleteNotesRequest) []interface{} {
	// Using generalized argument encoder
	args, err := argbuilder.EncodeRPCArgs(req, "[%project_id%, null, %note_ids%, [2]]")
	if err != nil {
		// Log error and return empty args as fallback
		// In production, this should be handled better
		return []interface{}{}
	}
	return args
}
