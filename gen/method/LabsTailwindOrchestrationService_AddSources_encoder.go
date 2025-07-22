package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeAddSourcesArgs encodes arguments for LabsTailwindOrchestrationService.AddSources
// RPC ID: izAoDd
// Argument format: [%sources%, %project_id%]
func EncodeAddSourcesArgs(req *notebooklmv1alpha1.AddSourceRequest) []interface{} {
	// AddSources encoding
	var sources []interface{}
	for _, src := range req.GetSources() {
		// Encode each source based on its type
		sources = append(sources, encodeSourceInput(src))
	}
	return []interface{}{sources, req.GetProjectId()}
}
