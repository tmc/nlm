package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// EncodeMutateProjectArgs encodes arguments for LabsTailwindOrchestrationService.MutateProject.
// RPC ID: s0tc2d
//
// Wire format verified against HAR captures (2026-04-25 nb-rename) — do not
// regenerate.
//
// The proto's `[%project_id%, %updates%]` format does not match the real wire
// shape. The browser sends:
//
//	[<project_id>, [<mutation>]]
//
// where <mutation> is a positional array whose slots select which property to
// change. Captured arms (1-indexed wire field numbers):
//
//	slot 4: [<unused>, <new_title>]              — rename title
//	slot 4: [<unused>, <unused>, <new_emoji>]    — change emoji
//	slot 8: [[], [<cover_preset_id>]]            — pick cover image
//	slot 10: [[<unused>×3, <description>]]       — set description / "creator notes"
//	slot 8 (api/SetChatConfig): [<chat config>]  — chat goal + length
//
// Title and emoji share slot 4 but at different sub-positions (1 vs 2). Other
// slots are independent — clients send one mutation arm at a time.
//
// Only Project.Title and Project.Emoji are covered by this encoder, since
// those are the fields the Project proto carries. Description and cover image
// have no Project proto fields; callers use api.Client convenience methods
// that build the args directly.
func EncodeMutateProjectArgs(req *notebooklmv1alpha1.MutateProjectRequest) []any {
	id := req.GetProjectId()
	var title, emoji string
	if upd := req.GetUpdates(); upd != nil {
		title = upd.GetTitle()
		emoji = upd.GetEmoji()
	}
	mutation := []any{nil, nil, nil, nil}
	if title != "" || emoji != "" {
		arm := []any{nil}
		if title != "" {
			arm = append(arm, title)
		} else {
			arm = append(arm, nil)
		}
		if emoji != "" {
			arm = append(arm, emoji)
		}
		mutation[3] = arm
	}
	return []any{id, []any{mutation}}
}

// MutateProjectDescriptionArgs builds the s0tc2d wire args for setting the
// notebook description ("creator notes"). The mutation goes in slot 10 (1-
// indexed) of the mutation submessage, wrapped one extra level:
//
//	[<project_id>, [[null×9, [[null, null, null, <description>]]]]]
//
// Wire format verified against HAR capture (2026-04-25).
func MutateProjectDescriptionArgs(projectID, description string) []any {
	mutation := []any{nil, nil, nil, nil, nil, nil, nil, nil, nil,
		[]any{[]any{nil, nil, nil, description}},
	}
	return []any{projectID, []any{mutation}}
}

// MutateProjectCoverArgs builds the s0tc2d wire args for selecting a notebook
// cover image. The mutation goes in slot 8 (1-indexed) of the mutation
// submessage:
//
//	[<project_id>, [[null×7, [[], [<cover_preset_id>]]]]]
//
// Wire format verified against HAR capture (2026-04-25). The captured value
// was preset_id=4; other valid IDs have not been observed.
func MutateProjectCoverArgs(projectID string, coverID int) []any {
	mutation := []any{nil, nil, nil, nil, nil, nil, nil,
		[]any{[]any{}, []any{coverID}},
	}
	return []any{projectID, []any{mutation}}
}

// MutateProjectCustomImageArgs builds the s0tc2d wire args for associating an
// already-uploaded custom image with the notebook. The image bytes must have
// been uploaded first via the CUSTOMIZATION resumable upload flow (see
// api.Client.UploadProjectCoverImage); the imageUUID here is the
// client-generated UUID submitted in the upload's IMAGE_UUID metadata field.
//
// The mutation goes in slot 10 (1-indexed) of the mutation submessage,
// distinct from the description arm which also lives in slot 10:
//
//	[<project_id>, [[null×9, [[], [[<imageUUID>]]]]]]
//
// Wire format verified against HAR capture (2026-04-25 nb-images).
func MutateProjectCustomImageArgs(projectID, imageUUID string) []any {
	mutation := []any{nil, nil, nil, nil, nil, nil, nil, nil, nil,
		[]any{[]any{}, []any{[]any{imageUUID}}},
	}
	return []any{projectID, []any{mutation}}
}
