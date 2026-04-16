package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// EncodeMutateSourceArgs encodes arguments for LabsTailwindOrchestrationService.MutateSource.
// RPC ID: b7Wfje
//
// Wire format verified against HAR capture — do not regenerate.
//
// The proto's `[%source_id%, %updates%]` format does not match the real wire
// format used by the NotebookLM web client. The browser sends a 3-argument
// positional array:
//
//	[null, [<source_id>], [[[[<title>]]]]]
//
// Position 0 is an unused leading slot. Position 1 is a SourceId submessage
// whose field 1 is the id string. Position 2 is a repeated list of
// mutations; each mutation is a oneof whose ChangePropertyMutation arm
// carries the new title at its field 1. Only title updates are supported by
// this encoder today — other Source fields are ignored.
func EncodeMutateSourceArgs(req *notebooklmv1alpha1.MutateSourceRequest) []interface{} {
	id := req.GetSourceId()
	var title string
	if upd := req.GetUpdates(); upd != nil {
		title = upd.GetTitle()
	}
	return []interface{}{
		nil,
		[]interface{}{id},
		[]interface{}{
			[]interface{}{
				[]interface{}{title},
			},
		},
	}
}
