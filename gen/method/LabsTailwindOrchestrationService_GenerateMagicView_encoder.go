package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeGenerateMagicViewArgs encodes arguments for LabsTailwindOrchestrationService.GenerateMagicView
// RPC ID: uK8f7c
// Argument format: [%project_id%, %source_ids%]
func EncodeGenerateMagicViewArgs(req *notebooklmv1alpha1.GenerateMagicViewRequest) []interface{} {
	var sourceIds []interface{}
	for _, sourceId := range req.GetSourceIds() {
		sourceIds = append(sourceIds, sourceId)
	}
	return []interface{}{
		req.GetProjectId(),
		sourceIds,
	}
}
