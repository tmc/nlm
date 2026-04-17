package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// EncodeDeleteArtifactArgs encodes arguments for
// LabsTailwindOrchestrationService.DeleteArtifact.
// RPC ID: V5N4be
//
// Wire format verified against HAR capture — do not regenerate.
//
// HAR source: NotebookLM web UI batchexecute capture.
// Captured 2026-04-07 against project
// 00000000-0000-4000-8000-000000000005. Two successive V5N4be POSTs
// (different artifact UUIDs, byte-identical options blob) returned empty
// arrays, followed by a gArtLc list-artifacts refresh — the standard
// post-delete UI pattern.
//
// Wire format:
//
//	[[2, null, null, [1, null, null, null, null, null, null, null, null, null, [1]],
//	  [[1, 4, 2, 3, 6, 5]]],
//	 "<artifact_uuid>"]
//
// The first positional is an opaque options descriptor. Every captured
// call uses the exact byte sequence below; it is treated as a literal
// until a second HAR capture shows it varying.
func EncodeDeleteArtifactArgs(req *notebooklmv1alpha1.DeleteArtifactRequest) []interface{} {
	options := []interface{}{
		2,
		nil,
		nil,
		[]interface{}{1, nil, nil, nil, nil, nil, nil, nil, nil, nil, []interface{}{1}},
		[]interface{}{[]interface{}{1, 4, 2, 3, 6, 5}},
	}
	return []interface{}{
		options,
		req.GetArtifactId(),
	}
}
