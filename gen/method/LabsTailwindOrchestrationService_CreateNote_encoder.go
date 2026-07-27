package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/argbuilder"
)

// GENERATION_BEHAVIOR: append

// EncodeCreateNoteArgs encodes arguments for LabsTailwindOrchestrationService.CreateNote
// RPC ID: CYK0Xb
// Argument format: [%project_id%, "", %note_type%, null, "New Note", null, [2, null, [1], [1, null, null, null, null, null, null, null, null, null, [1, 3]]]]
func EncodeCreateNoteArgs(req *notebooklmv1alpha1.CreateNoteRequest) []interface{} {
	// Using generalized argument encoder. printf %q emits a properly escaped Go
	// string literal so arg_formats containing quotes (e.g. "New Note") stay valid.
	args, err := argbuilder.EncodeRPCArgs(req, "[%project_id%, \"\", %note_type%, null, \"New Note\", null, [2, null, [1], [1, null, null, null, null, null, null, null, null, null, [1, 3]]]]")
	if err != nil {
		// Log error and return empty args as fallback
		// In production, this should be handled better
		return []interface{}{}
	}
	return args
}
