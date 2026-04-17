package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
)

// EncodeRenameArtifactArgs encodes arguments for
// LabsTailwindOrchestrationService.RenameArtifact.
// RPC ID: rc3d8d
//
// Wire format verified by live end-to-end success — do not regenerate.
//
// Provenance: the hand-crafted shape below has been running in production
// from api.Client.RenameArtifact (pre-2026-04-16) and renames artifacts
// successfully against the NotebookLM backend (phase1-triage row #20).
// No passive HAR capture was available at the time of this encoder move
// (the on-hand 2026-04-07 capture covers V5N4be delete-artifact but not
// rc3d8d rename-artifact; the in-queue CDP session was unavailable). If a
// future HAR capture shows a different shape, update both this encoder
// and the testdata fixture together.
//
// Wire format:
//
//	[["<artifact_id>", "<new_title>"], [["title"]]]
//
// The second positional is a field-mask-style list: an outer list
// containing a single inner list naming the fields being mutated. Today
// only "title" is writable via rename, so it is always [["title"]].
func EncodeRenameArtifactArgs(req *notebooklmv1alpha1.RenameArtifactRequest) []interface{} {
	return []interface{}{
		[]interface{}{req.GetArtifactId(), req.GetNewTitle()},
		[]interface{}{[]interface{}{"title"}},
	}
}
