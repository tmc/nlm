package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeGetProjectDetailsArgs encodes arguments for LabsTailwindSharingService.GetProjectDetails
// RPC ID: JFMDGd
//
// Wire format: [share_id, [2]]
//
//	Field 1: share_id (project UUID)
//	Field 2: ProjectContext {field 1: 2}
func EncodeGetProjectDetailsArgs(req *notebooklmv1alpha1.GetProjectDetailsRequest) []interface{} {
	return []interface{}{req.GetShareId(), []interface{}{2}}
}
