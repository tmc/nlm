package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/argbuilder"
)

// GENERATION_BEHAVIOR: append

// EncodeLogInteractionEventArgs encodes arguments for InteractionEventService.LogInteractionEvent
// RPC ID: HpN0Ub
// Argument format: [%context%, %event_2%, %event_3%, %event_4%]
func EncodeLogInteractionEventArgs(req *notebooklmv1alpha1.LogInteractionEventRequest) []interface{} {
	// Using generalized argument encoder. printf %q emits a properly escaped Go
	// string literal so arg_formats containing quotes (e.g. "New Note") stay valid.
	args, err := argbuilder.EncodeRPCArgs(req, "[%context%, %event_2%, %event_3%, %event_4%]")
	if err != nil {
		// Log error and return empty args as fallback
		// In production, this should be handled better
		return []interface{}{}
	}
	return args
}
