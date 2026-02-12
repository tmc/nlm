package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// GENERATION_BEHAVIOR: append

// EncodeMutateSourceArgs encodes arguments for LabsTailwindOrchestrationService.MutateSource
// RPC ID: b7Wfje
//
// Wire format: [null, ["sourceId"], [update1, update2, ...]]
//   Field 2: Ru source reference {field 1: source ID}
//   Field 3: repeated CJa updates, each with oneof field 1 = DJa{field 1: new name}
//
// For rename: [null, ["sourceId"], [["newName"]]]
func EncodeMutateSourceArgs(req *notebooklmv1alpha1.MutateSourceRequest) []interface{} {
	// Source reference: Ru{field 1: sourceId}
	sourceRef := []interface{}{req.GetSourceId()}

	// Build updates — currently only supports rename (name update)
	var updates []interface{}
	if req.GetUpdates() != nil {
		name := req.GetUpdates().GetTitle()
		if name != "" {
			// CJa{oneof field 1: DJa{field 1: name}} = [["newName"]]
			updates = append(updates, []interface{}{[]interface{}{name}})
		}
	}

	return []interface{}{
		nil,       // field 1: gap
		sourceRef, // field 2: Ru source reference
		updates,   // field 3: repeated updates
	}
}
