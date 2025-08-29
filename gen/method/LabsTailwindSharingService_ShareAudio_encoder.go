package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeShareAudioArgs encodes arguments for LabsTailwindSharingService.ShareAudio
// RPC ID: RGP97b
// Argument format: [%share_options%, %project_id%]
func EncodeShareAudioArgs(req *notebooklmv1alpha1.ShareAudioRequest) []interface{} {
	var shareOptions []interface{}
	for _, option := range req.GetShareOptions() {
		shareOptions = append(shareOptions, option)
	}
	return []interface{}{
		shareOptions,
		req.GetProjectId(),
	}
}
