package method

import (
	notebooklmv1alpha1 "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/rpc/argbuilder"
)

// GENERATION_BEHAVIOR: append

// EncodeRefreshSourceArgs encodes arguments for LabsTailwindOrchestrationService.RefreshSource.
// RPC ID: FLmJqe
//
// BLOCKED on HAR capture. CheckSourceFreshness (yR9Yof) uses
// [null, [source_id], [4]] and is live-verified working; applying the same
// shape to RefreshSource returns `API error 3` against the live server
// (tested 2026-04-17 against note, text, and youtube sources). RefreshSource
// almost certainly needs a different positional shape — possibly including
// the project_id, possibly a mutation-envelope like MutateSource
// ([null, [source_id], [[[[...]]]]]). Do not guess; see
// docs/dev/har-capture-queue.md P1.4 for the capture requirement.
//
// Current behavior: falls back to argbuilder stub, which ALSO fails. Kept
// non-broken-compile so the CLI surfaces a sensible "invalid argument" error
// instead of panicking.
func EncodeRefreshSourceArgs(req *notebooklmv1alpha1.RefreshSourceRequest) []interface{} {
	args, err := argbuilder.EncodeRPCArgs(req, "[%source_id%]")
	if err != nil {
		return []interface{}{}
	}
	return args
}
