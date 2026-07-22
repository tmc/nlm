package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/rpc/argbuilder"
)

// GENERATION_BEHAVIOR: append

// EncodeCreateNoteArgs encodes arguments for LabsTailwindOrchestrationService.CreateNote
// RPC ID: CYK0Xb
// Argument format: [%project_id%, "", [1], null, "New Note", null, [2]]
func EncodeCreateNoteArgs(req *notebooklmv1alpha1.CreateNoteRequest) []interface{} {
	// Using generalized argument encoder. printf %q emits a properly escaped Go
	// string literal so arg_formats containing quotes (e.g. "New Note") stay valid.
	args, err := argbuilder.EncodeRPCArgs(req, "[%project_id%, \"\", [1], null, \"New Note\", null, [2]]")
	if err != nil {
		// Log error and return empty args as fallback
		// In production, this should be handled better
		return []interface{}{}
	}
	return args
}
