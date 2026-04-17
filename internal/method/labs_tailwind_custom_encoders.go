package method

import (
	"time"

	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// This file contains hand-coded encoders that replace broken argbuilder-based
// encoders. Each wire format was verified against HAR captures from the
// NotebookLM web UI.

// EncodePublishGuidebookArgsV2 encodes arguments for LabsTailwindGuidebooksService.PublishGuidebook.
// RPC ID: R6smae (service client uses this), HAR captured as khqZz.
//
// Wire format verified against HAR capture:
//
//	[[], null, null, "<guidebook_id>", 20]
func EncodePublishGuidebookArgsV2(req *notebooklmv1alpha1.PublishGuidebookRequest) []interface{} {
	return []interface{}{
		[]interface{}{},      // field 1: empty settings array
		nil,                  // field 2: null
		nil,                  // field 3: null
		req.GetGuidebookId(), // field 4: guidebook ID
		20,                   // field 5: publish mode (20 = public)
	}
}

// EncodeShareGuidebookArgsV2 encodes arguments for LabsTailwindGuidebooksService.ShareGuidebook.
// RPC ID: OTl0K (service client uses this), HAR captured as sqTeoe.
//
// Wire format verified against HAR capture:
//
//	[[2, null, null, [1,null,null,null,null,null,null,null,null,null,[1]], [[1,4,2,3,6,5]]], null, 1]
//
// The request includes sharing configuration that controls what content types
// are available in the guidebook (audio types, video types, slide types, text types).
func EncodeShareGuidebookArgsV2(req *notebooklmv1alpha1.ShareGuidebookRequest) []interface{} {
	// Sharing config: audio style options [1], content type ordering [1,4,2,3,6,5]
	sharingConfig := []interface{}{
		2,   // field 1: sharing mode (2 = public link)
		nil, // field 2: null
		nil, // field 3: null
		[]interface{}{1, nil, nil, nil, nil, nil, nil, nil, nil, nil, []interface{}{1}}, // field 4: audio style config
		[]interface{}{[]interface{}{1, 4, 2, 3, 6, 5}},                                  // field 5: content type ordering
	}
	return []interface{}{
		sharingConfig, // field 1: sharing configuration
		nil,           // field 2: null
		1,             // field 3: flag (1 = enabled)
	}
}

// EncodeDeleteGuidebookArgsV2 encodes arguments for LabsTailwindGuidebooksService.DeleteGuidebook.
// RPC ID: ARGkVc (service client uses this), HAR captured as ozz5Z.
//
// Wire format verified against HAR capture:
//
//	[[[[null, "<id>", <int>], [null,null,null,null,null,null,null,null,null,[null,null,2]], 1]]]
//
// The guidebook ID is a numeric string, not a UUID. The integer field (627)
// appears to be a version or sequence number.
func EncodeDeleteGuidebookArgsV2(req *notebooklmv1alpha1.DeleteGuidebookRequest) []interface{} {
	// Guidebook reference with context suffix
	guidebookRef := []interface{}{
		nil,                  // field 1: null
		req.GetGuidebookId(), // field 2: guidebook ID
		nil,                  // field 3: version/sequence (omitted, server infers)
	}
	contextSuffix := []interface{}{
		nil, nil, nil, nil, nil, nil, nil, nil, nil,
		[]interface{}{nil, nil, 2}, // field 10: project context marker
	}
	entry := []interface{}{guidebookRef, contextSuffix, 1}
	return []interface{}{
		[]interface{}{entry}, // wrapped in repeated field
	}
}

// EncodeGuidebookGenerateAnswerArgsV2 encodes arguments for
// LabsTailwindGuidebooksService.GuidebookGenerateAnswer.
// RPC ID: itA0pc (service client uses this), HAR captured as eyWvXc.
//
// Wire format verified against HAR capture:
//
//	["<sdp_or_question>", "<guidebook_id>", 0, "<notebook_id>"]
//
// The HAR shows this is used for interactive audio sessions (WebRTC SDP),
// but the same format works for text questions. Field 3 is a mode flag
// (0 = default).
func EncodeGuidebookGenerateAnswerArgsV2(req *notebooklmv1alpha1.GuidebookGenerateAnswerRequest) []interface{} {
	return []interface{}{
		req.GetQuestion(),    // field 1: question or SDP offer
		req.GetGuidebookId(), // field 2: guidebook ID
		0,                    // field 3: mode flag
		"",                   // field 4: notebook ID (empty for standalone questions)
	}
}

// EncodeDeleteAudioOverviewArgsV2 encodes arguments for
// LabsTailwindOrchestrationService.DeleteAudioOverview.
// RPC ID: sJDbic (service client uses this), HAR captured as hizoJc.
//
// Wire format verified against HAR capture:
//
//	[["<project_id>"], [2], [2]]
//
// Note: the first field wraps the project/audio ID in an array.
// The two [2] fields are ProjectContext markers.
func EncodeDeleteAudioOverviewArgsV2(req *notebooklmv1alpha1.DeleteAudioOverviewRequest) []interface{} {
	return []interface{}{
		[]interface{}{req.GetProjectId()}, // field 1: project/audio ID wrapped
		[]interface{}{2},                  // field 2: ProjectContext
		[]interface{}{2},                  // field 3: ProjectContext (repeated)
	}
}

// EncodeShareAudioArgsV2 encodes arguments for LabsTailwindSharingService.ShareAudio.
// RPC ID: RGP97b (service client uses this), HAR captured as hPTbtc.
//
// Wire format verified against HAR capture:
//
//	[[], null, "<project_id>", 20]
func EncodeShareAudioArgsV2(req *notebooklmv1alpha1.ShareAudioRequest) []interface{} {
	return []interface{}{
		[]interface{}{},    // field 1: share options (empty = default)
		nil,                // field 2: null
		req.GetProjectId(), // field 3: project ID
		20,                 // field 4: sharing mode (20 = public)
	}
}

// EncodeGetProjectAnalyticsArgsV2 encodes arguments for
// LabsTailwindOrchestrationService.GetProjectAnalytics.
// RPC ID: AUrzMb (service client uses this), HAR captured as cFji9.
//
// Wire format verified against HAR capture:
//
//	["<project_id>", null, [<timestamp_seconds>, <timestamp_nanos>], [2]]
func EncodeGetProjectAnalyticsArgsV2(req *notebooklmv1alpha1.GetProjectAnalyticsRequest) []interface{} {
	// Use current time as the analytics timestamp
	now := time.Now()
	timestamp := []interface{}{
		now.Unix(),
		int64(now.Nanosecond()),
	}
	return []interface{}{
		req.GetProjectId(), // field 1: project ID
		nil,                // field 2: null
		timestamp,          // field 3: timestamp [seconds, nanos]
		[]interface{}{2},   // field 4: ProjectContext
	}
}
