package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/rpc/argbuilder"
)

// GENERATION_BEHAVIOR: append

// EncodeShareAudioArgs encodes arguments for LabsTailwindSharingService.ShareAudio
// RPC ID: RGP97b
// Argument format: [%share_options%, %project_id%]
func EncodeShareAudioArgs(req *notebooklmv1alpha1.ShareAudioRequest) []interface{} {
	// Using generalized argument encoder
	args, err := argbuilder.EncodeRPCArgs(req, "[%share_options%, %project_id%]")
	if err != nil {
		// Log error and return empty args as fallback
		// In production, this should be handled better
		return []interface{}{}
	}
	return args
}
