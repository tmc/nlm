package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeShareProjectArgs encodes arguments for LabsTailwindSharingService.ShareProject
// RPC ID: QDyure
// Argument format: [%project_id%, %settings%]
func EncodeShareProjectArgs(req *notebooklmv1alpha1.ShareProjectRequest) []interface{} {
	return []interface{}{
		req.GetProjectId(),
		req.GetSettings(),
	}
}
