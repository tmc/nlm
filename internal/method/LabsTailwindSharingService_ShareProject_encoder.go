package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// EncodeShareProjectArgs encodes arguments for LabsTailwindSharingService.ShareProject.
// RPC ID: QDyure
//
// Wire format verified against HAR capture — do not regenerate.
//
// The proto's `[%project_id%, %settings%]` format is not what the browser
// actually sends. The real shape is:
//
//	[[[<project_id>, null, <link_settings>, [0, ""]]], <M3 flag>, null, [2]]
//
// Position 0 is a single-element list wrapping a YM message:
//   - field 1: project_id
//   - field 3: Uzb submessage — link settings: [enabled, visibility]
//     [1, 0] = link sharing enabled, private (invite-only)
//     [1, 1] = link sharing enabled, public
//   - field 4: [0, ""] — unused tail
//
// Position 1 is the M3 flag (always 1 in captured traffic).
// Position 2 is a gap (null).
// Position 3 is ProjectContext [2].
//
// Derived from the JS mAb function in the NotebookLM web client and
// verified against captured share traffic.
func EncodeShareProjectArgs(req *notebooklmv1alpha1.ShareProjectRequest) []interface{} {
	visibility := 0 // private (invite-only)
	if s := req.GetSettings(); s != nil && s.GetIsPublic() {
		visibility = 1
	}
	return []interface{}{
		[]interface{}{
			[]interface{}{
				req.GetProjectId(),
				nil,
				[]interface{}{1, visibility},
				[]interface{}{0, ""},
			},
		},
		1,
		nil,
		[]interface{}{2},
	}
}
