package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeDeleteSourcesArgs encodes arguments for LabsTailwindOrchestrationService.DeleteSources
// RPC ID: tGMBJ
// Argument format: [[%source_ids%]]
func EncodeDeleteSourcesArgs(req *notebooklmv1alpha1.DeleteSourcesRequest) []interface{} {
	// Nested source IDs encoding
	return []interface{}{[]interface{}{req.GetSourceIds()}}
}
