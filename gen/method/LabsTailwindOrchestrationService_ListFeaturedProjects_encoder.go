package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeListFeaturedProjectsArgs encodes arguments for LabsTailwindOrchestrationService.ListFeaturedProjects
// RPC ID: ub2Bae (was nS9Qlc which is wrong)
//
// Wire format: [[2]]
//   Field 1: ProjectContext {field 1: 2}
func EncodeListFeaturedProjectsArgs(req *notebooklmv1alpha1.ListFeaturedProjectsRequest) []interface{} {
	projectContext := []interface{}{2}
	return []interface{}{projectContext}
}
