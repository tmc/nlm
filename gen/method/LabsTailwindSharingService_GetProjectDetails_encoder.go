package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeGetProjectDetailsArgs encodes arguments for LabsTailwindSharingService.GetProjectDetails
// RPC ID: JFMDGd
// Argument format: [%share_id%]
func EncodeGetProjectDetailsArgs(req *notebooklmv1alpha1.GetProjectDetailsRequest) []interface{} {
	return []interface{}{
		req.GetShareId(),
	}
}
