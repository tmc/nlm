package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeMutateSourceArgs encodes arguments for LabsTailwindOrchestrationService.MutateSource
// RPC ID: b7Wfje
//
// Wire format from capture: [null, ["source-id"], [[["new title"]]]]
//   - Position 0: null
//   - Position 1: [source_id] — SourceId message as single-element array
//   - Position 2: [[[title]]] — title wrapped in triple nesting
func EncodeMutateSourceArgs(req *notebooklmv1alpha1.MutateSourceRequest) []interface{} {
	// Wire format verified against HAR capture — do not regenerate.
	updates := req.GetUpdates()
	args := []interface{}{
		nil,
		[]interface{}{req.GetSourceId()},
	}
	if updates != nil && updates.GetTitle() != "" {
		args = append(args, []interface{}{[]interface{}{[]interface{}{updates.GetTitle()}}})
	}
	return args
}
