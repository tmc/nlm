package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/rpc/argbuilder"
)

// GENERATION_BEHAVIOR: append

// EncodeGenerateNotebookGuideArgs encodes arguments for LabsTailwindOrchestrationService.GenerateNotebookGuide
// RPC ID: VfAZjd
// Argument format: [%project_id%]
func EncodeGenerateNotebookGuideArgs(req *notebooklmv1alpha1.GenerateNotebookGuideRequest) []interface{} {
	// Using generalized argument encoder
	args, err := argbuilder.EncodeRPCArgs(req, "[%project_id%]")
	if err != nil {
		// Log error and return empty args as fallback
		// In production, this should be handled better
		return []interface{}{}
	}
	return args
}
