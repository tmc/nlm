package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/rpc/argbuilder"
)

// GENERATION_BEHAVIOR: append

// EncodeGenerateMagicViewArgs encodes arguments for LabsTailwindOrchestrationService.GenerateMagicView
// RPC ID: uK8f7c
// Argument format: [%project_id%, %source_ids%]
func EncodeGenerateMagicViewArgs(req *notebooklmv1alpha1.GenerateMagicViewRequest) []interface{} {
	// Using generalized argument encoder
	args, err := argbuilder.EncodeRPCArgs(req, "[%project_id%, %source_ids%]")
	if err != nil {
		// Log error and return empty args as fallback
		// In production, this should be handled better
		return []interface{}{}
	}
	return args
}
