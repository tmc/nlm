---
title: Remaining Gaps and Audit Notes
date: 2026-04-21
---

# nlm Remaining Gaps

This file was re-audited against the current tree on 2026-04-21. It tracks
only gaps that still matter after checking the live CLI, API, and MCP code
paths.

### 1. Analytics remains experimental and misleading

`AUrzMb` returns time-series metrics, but the generated `ProjectAnalytics`
shape still expects scalar counts. The command is intentionally hidden
behind `--experimental` in `cmd/nlm/commands.go`, and `cmd/nlm/main.go`
prints a warning that the output is unreliable.

Status: open, not HAR-blocked.

Next step: redesign the response model and CLI UX around metric series, not
another encoder tweak.

### 2. Video download is still manual-fallback only

`video download` still relies on the direct-RPC-only path in
`internal/notebooklm/api/client.go`. When the API response does not expose
direct media bytes or a CDN URL, the command now fails explicitly with a
manual-browser fallback instead of probing speculative RPC shapes.

Status: open. This is not an MCP gap. HAR would help automation, but the
current user-visible limitation is the lack of a verified automated download
path plus
CDN browser-auth requirements.

### 3. Weakly verified encoder paths still exist

Dead-path RPCs such as `xpWGLf`, `lCjAd`, and `BeTrYd` still exist in
codegen output / compatibility paths even though the current CLI does
not route through them. A few low-use argbuilder encoders are also still
weakly verified:

- `SubmitFeedback` (`uNyJKe`) works in practice but still uses the generic
  `[%project_id%, %feedback_type%, %feedback_text%]` shape.
- `DeleteNotes` (`AH0mwd`) works in practice but is not pinned by a
  HAR-backed encoder test.
- `GenerateFreeFormStreamed` exists as a gRPC-Web chat path in
  `internal/notebooklm/api`; the batchexecute method encoder is not a live
  CLI path.

Status: low priority cleanup / verification work.

### 4. `artifact get` returns API endpoint errors

`nlm artifact get <artifact-id>` returns API endpoint errors against the
live service. The RPC wire format has not been re-captured since the
2026-04-07 session and the current encoder may not match. May need a
direct-RPC fallback or a fresh HAR to derive the right shape.

Status: open, HAR helpful but not strictly required.

### 5. `chat config` server semantics unverified

`nlm chat config <id> <setting> [value]` rides on `MutateProject`
(`s0tc2d`) to apply chat goal/length settings. The CLI accepts
`goal default`, `goal custom "prompt"`, and `length default|longer|shorter`,
but none of these paths have been verified end-to-end against the live
service, and the `ChatGoal` enum values may not match server expectations.

Status: open. Low usage; verify when there is a real caller.

### 6. Auth-expiry mid-session gives unclear errors

When the session cookies expire during a long-running command, the user
sees an opaque error rather than a clear "re-run `nlm auth`" prompt.
Consider auto-refresh on 401/Unauthenticated responses, or at minimum
detect the failure mode and surface a targeted message.

Status: open, UX polish.

## Truly HAR-Blocked

### 1. `izAoDd` drag-drop bulk add shape

The generic bulk-add RPC still has no verified drag-drop capture. This no
longer blocks the main programmatic bulk-import use case, because
`nlm research --import` already uses the HAR-verified `LBwxtb`
bulk-import variant instead.

Status: HAR-blocked, low value until there is a real CLI caller.

### 2. Deep-research session state `6`

The active parser in `api.Client.pollResearch` safely treats unknown
states as still-running, but the semantics of observed state `6`
remain unknown.

Status: HAR-blocked for semantics only. The current fallback is safe.

## Next Work

1. Keep `analytics` experimental until its proto and CLI UX are redesigned.
2. Decide whether `video download` should keep the current manual-fallback
   UX or get a real CDN capture and a browser-assisted path.
3. Remove dead generated RPC stubs so future audits do not
   mistake them for live command paths.
4. Re-capture `artifact get` against the live service and fix the encoder.
5. Verify `chat config` end-to-end (or hide it until there is a real caller).
6. Capture `izAoDd` only if a real bulk-add CLI caller is introduced.
