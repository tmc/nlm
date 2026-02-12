package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeShareProjectArgs encodes arguments for LabsTailwindSharingService.ShareProject
// RPC ID: QDyure
//
// Wire format from JS analysis (mAb function):
//   [[[projectId, emailRoles, linkSettings, notification]], boolFlag, null, [2]]
//
// Field 1: repeated YM share targets
//   YM field 1: string (project ID)
//   YM field 2: repeated Vzb (email-role pairs) — optional
//   YM field 3: Uzb (link sharing settings) — {field 1: bool (enabled)}
//   YM field 4: xw (notification settings) — optional
// Field 2: bool (M3 flag)
// Field 4: ProjectContext [2]
func EncodeShareProjectArgs(req *notebooklmv1alpha1.ShareProjectRequest) []interface{} {
	settings := req.GetSettings()

	// Build link sharing settings: Uzb{field 1: is_public}
	var linkSettings interface{}
	if settings != nil {
		linkSettings = []interface{}{settings.GetIsPublic()}
	} else {
		linkSettings = []interface{}{true} // default to public
	}

	// Build YM share target
	shareTarget := []interface{}{
		req.GetProjectId(), // field 1: project ID
		nil,                // field 2: email-role pairs (not used for link sharing)
		linkSettings,       // field 3: Uzb link sharing settings
	}

	// ProjectContext
	projectContext := []interface{}{2}

	return []interface{}{
		[]interface{}{shareTarget}, // field 1: repeated YM
		true,                       // field 2: M3 flag
		nil,                        // field 3: gap
		projectContext,             // field 4: ProjectContext
	}
}
