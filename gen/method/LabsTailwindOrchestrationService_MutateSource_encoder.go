package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeMutateSourceArgs encodes arguments for LabsTailwindOrchestrationService.MutateSource
// RPC ID: b7Wfje
// Argument format: [%source_id%, %updates%]
func EncodeMutateSourceArgs(req *notebooklmv1alpha1.MutateSourceRequest) []interface{} {
	// MutateSource encoding
	return []interface{}{req.GetSourceId(), encodeSourceUpdates(req.GetUpdates())}
}
