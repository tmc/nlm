package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/rpc/argbuilder"
)

// GENERATION_BEHAVIOR: append

// EncodeGetProjectDetailsArgs encodes arguments for LabsTailwindSharingService.GetProjectDetails
// RPC ID: JFMDGd
// Argument format: [%share_id%]
func EncodeGetProjectDetailsArgs(req *notebooklmv1alpha1.GetProjectDetailsRequest) []interface{} {
	// Using generalized argument encoder
	args, err := argbuilder.EncodeRPCArgs(req, "[%share_id%]")
	if err != nil {
		// Log error and return empty args as fallback
		// In production, this should be handled better
		return []interface{}{}
	}
	return args
}
