package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeShareProjectArgs encodes arguments for LabsTailwindSharingService.ShareProject
// RPC ID: QDyure
//
// HAR-verified wire format:
//
//	[[[projectId, null, [1, accessLevel], [0, ""]]], 1, null, [2]]
//
// Field 1: repeated YM share targets
// Field 2: int (not bool)
// Field 3: null
// Field 4: ProjectContext [2]
func EncodeShareProjectArgs(req *notebooklmv1alpha1.ShareProjectRequest) []interface{} {
	// Wire format verified against HAR capture — do not regenerate.
	settings := req.GetSettings()

	// Build link sharing settings: [1, accessLevel]
	// 1 = link sharing enabled; accessLevel: 0=private, 1=public
	accessLevel := 0
	if settings != nil && settings.GetIsPublic() {
		accessLevel = 1
	}
	linkSettings := []interface{}{1, accessLevel}

	// Build notification settings
	notification := []interface{}{0, ""}

	// Build YM share target
	shareTarget := []interface{}{
		req.GetProjectId(), // field 1: project ID
		nil,                // field 2: email-role pairs (not used for link sharing)
		linkSettings,       // field 3: [1, accessLevel]
		notification,       // field 4: notification settings
	}

	// ProjectContext
	projectContext := []interface{}{2}

	return []interface{}{
		[]interface{}{shareTarget}, // field 1: repeated YM
		1,                          // field 2: int (not bool)
		nil,                        // field 3: gap
		projectContext,             // field 4: ProjectContext
	}
}
