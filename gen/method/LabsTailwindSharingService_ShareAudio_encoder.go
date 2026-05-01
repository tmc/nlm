package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeShareAudioArgs encodes arguments for LabsTailwindSharingService.ShareAudio
// RPC ID: RGP97b
// Wire format verified against HAR capture (hPTbtc):
//
//	[[], null, "<project_id>", 20]
func EncodeShareAudioArgs(req *notebooklmv1alpha1.ShareAudioRequest) []interface{} {
	return EncodeShareAudioArgsV2(req)
}
