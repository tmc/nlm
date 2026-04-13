package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// Wire format verified against HAR capture — do not regenerate.

// EncodeDiscoverSourcesArgs encodes arguments for LabsTailwindOrchestrationService.DiscoverSources
// RPC ID: qXyaNe
//
// Wire format: [project_id, query, [2]]
func EncodeDiscoverSourcesArgs(req *notebooklmv1alpha1.DiscoverSourcesRequest) []interface{} {
	return []interface{}{
		req.GetProjectId(),
		req.GetQuery(),
		[]interface{}{2},
	}
}
