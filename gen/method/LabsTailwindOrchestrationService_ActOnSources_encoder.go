package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/argbuilder"
)

// GENERATION_BEHAVIOR: append

// EncodeActOnSourcesArgs encodes arguments for LabsTailwindOrchestrationService.ActOnSources
// RPC ID: yyryJe
// Argument format: [%source_groups%, %options%, null, null, null, null, null, %chat_options%, %prompt_history%, %conversation_id%]
func EncodeActOnSourcesArgs(req *notebooklmv1alpha1.ActOnSourcesRequest) []interface{} {
	// Using generalized argument encoder. printf %q emits a properly escaped Go
	// string literal so arg_formats containing quotes (e.g. "New Note") stay valid.
	args, err := argbuilder.EncodeRPCArgs(req, "[%source_groups%, %options%, null, null, null, null, null, %chat_options%, %prompt_history%, %conversation_id%]")
	if err != nil {
		// Log error and return empty args as fallback
		// In production, this should be handled better
		return []interface{}{}
	}
	return args
}
