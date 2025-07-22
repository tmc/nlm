package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeActOnSourcesArgs encodes arguments for LabsTailwindOrchestrationService.ActOnSources
// RPC ID: yyryJe
// Argument format: [%project_id%, %action%, %source_ids%]
func EncodeActOnSourcesArgs(req *notebooklmv1alpha1.ActOnSourcesRequest) []interface{} {
	// ActOnSources encoding
	return []interface{}{req.GetProjectId(), req.GetAction(), req.GetSourceIds()}
}
