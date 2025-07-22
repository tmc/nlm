package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeLoadSourceArgs encodes arguments for LabsTailwindOrchestrationService.LoadSource
// RPC ID: hizoJc
// Argument format: [%source_id%]
func EncodeLoadSourceArgs(req *notebooklmv1alpha1.LoadSourceRequest) []interface{} {
	// Single source ID encoding
	return []interface{}{req.GetSourceId()}
}
