---
title: Remaining Gaps and Audit Notes
date: 2026-07-27
---

# nlm Remaining Gaps

This file was re-audited against the current tree and the verified competitive
analysis on 2026-07-27. The items below are real gaps after checking the live
CLI, API, MCP, and protocol-modeling paths. They are not claims inferred from
missing documentation.

## Capabilities That Are Already Present

These are not gaps:

- `nlm auth login` drives Chrome, Brave, or Edge headlessly and extracts the
  token and cookies from an already signed-in profile without copy-paste.
- `nlm chat` is a streaming interactive client with persistent on-disk
  sessions, history, and slash commands.
- Chat and generation commands select sources by UUID, title regular
  expression, and server-side label.
- `nlm research --md` emits the deep-research Markdown report.
- `create_note` and `add_source_text` provide MCP context injection;
  `start_deep_research` and `poll_deep_research` provide asynchronous research.
- Directory-tree sync, RichDocument note rendering, and txtar citation
  `file:line` resolution are implemented.

The authentication constraint is narrower: browser login harvests credentials
from a profile already signed into Google. It does not provide unattended SSO
on a fresh machine.

## Confirmed Product Gaps

### 1. Broad artifact export

Slide decks can be downloaded as PDF or PPTX, and audio/video have dedicated
download paths. There is no general artifact-export command and no structured
flashcard, quiz, report, data-table, or mind-map export.

Flashcard card content and mind-map structure are not sufficiently modeled to
invent exporters. Any work on those formats starts with a real capture and a
lossless corpus gate.

Status: high priority, capture-blocked for flashcard cards and mind-map
structure. Rendered server outputs may support narrower exports without new
wire modeling and should be assessed separately.

### 2. Proactive credential refresh

The CLI already has a reactive recovery path: after an authentication-shaped
failure, commands with a cached browser profile silently re-run browser
authentication and retry once. Environment-only CI credentials fail directly
because no local browser profile can refresh them.

The older proactive `TokenManager` path is not functional with real `SNlM0e`
tokens: `ParseAuthToken` expects an artificial `token:timestamp` shape, and the
signaler request does not capture and install a renewed NotebookLM token in the
live client.

Status: reactive browser-profile recovery works; proactive refresh remains
open. Prefer the verified browser re-harvest path unless a capture proves that
the signaler endpoint can mint the needed token.

### 3. Deep-research inline citations

`nlm research --md` emits the report and a structured citation list, but does
not weave those citations into the Markdown as links or footnotes.

Status: open, no new wire modeling required.

### 4. MCP watch and progress

The stdio MCP server exposes 38 tools, including research start and poll, but
has no blocking watch tool and emits no MCP progress notifications. It also
lacks HTTP/SSE transport, in-MCP authentication recovery, and a local
file-path injection tool.

Status: watch/progress is the first MCP gap to close. Text injection already
works through `add_source_text`; do not describe context injection itself as
missing.

### 5. Agent onboarding and competitive breadth

The in-repo skill is concise and delegates to current `--help`, but there is no
one-shot `nlm --ai` documentation dump or skill installer. The MCP surface also
omits some sharing, batch, and cross-notebook operations exposed by broader
competitors. AI-assisted auto-labeling is not implemented.

Status: open. Preserve the lean skill design; improve discovery and
distribution rather than copying a kitchen-sink command surface.

## Lower-Priority Protocol and API Work

### 1. Generated analytics proto remains scalar

`AUrzMb` returns time-series metrics, but the generated `ProjectAnalytics`
shape still expects scalar counts. The public API and CLI now bypass that
generated response model and parse the fixture-backed time-series shape into
typed metric rows.

Status: low-priority generated-proto cleanup, not HAR-blocked.

Next step: decide whether the generated proto should grow metric-series
messages or whether AUrzMb should remain a typed API-only path.

### 2. Video download is still manual-fallback only

`video download` still relies on the direct-RPC-only path in
`internal/notebooklm/api/client.go`. When the API response does not expose
direct media bytes or a CDN URL, the command now fails explicitly with a
manual-browser fallback instead of probing speculative RPC shapes.

Status: open. This is not an MCP gap. HAR would help automation, but the
current user-visible limitation is the lack of a verified automated download
path plus CDN browser-auth requirements. The CLI fallback prints the
NotebookLM notebook URL so the user can finish the download in a browser.

### 3. Weakly verified encoder paths still exist

Dead-path RPCs such as `xpWGLf`, `lCjAd`, and `BeTrYd` still exist in
codegen output / compatibility paths even though the current CLI does
not route through them. A few low-use argbuilder encoders are also still
weakly verified:

- `SubmitFeedback` (`uNyJKe`) works in practice but still uses the generic
  `[%project_id%, %feedback_type%, %feedback_text%]` shape.
- `DeleteNotes` (`AH0mwd`) works in practice but is not pinned by a
  HAR-backed encoder test.
- `GenerateNotebookGuide` (`VfAZjd`) has a hand-written encoder that emits
  `[%project_id%, %guide_type%]`, but both outline and mind-map variants
  still need HAR capture before the guard comment can call the shape verified.
- `GenerateFreeFormStreamed` exists as a gRPC-Web chat path in
  `internal/notebooklm/api`; the batchexecute method encoder is not a live
  CLI path.

Status: low priority cleanup / verification work.

### 4. `artifact get` direct path remains unverified

`nlm artifact get <artifact-id>` now tries the JS-bundle-canonical
`v9rmvd` direct RPC first, then falls back to scanning
`ListRecentlyViewedProjects` plus `ListArtifacts` when the direct path
fails or returns an unparsable response. That keeps the user-visible command
on the same list-scan behavior the CLI used before the direct probe.

Status: open for direct-path verification only. A fresh HAR would confirm
whether `v9rmvd` is callable on the live service or should remain a
best-effort fast path behind the list-scan fallback.

### 5. `chat config` server semantics unverified

`nlm chat config <id> <setting> [value]` rides on `MutateProject`
(`s0tc2d`) to apply chat goal/length settings. The CLI accepts
`goal default`, `goal custom "prompt"`, and `length default|longer|shorter`,
but none of these paths have been verified end-to-end against the live
service, and the `ChatGoal` enum values may not match server expectations.

Status: open. Low usage; verify when there is a real caller.

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

1. Capture and lossless-verify flashcard/quiz payloads before modeling card
   content or adding structured export.
2. Add deep-research inline Markdown citations.
3. Add MCP research watch/progress behavior.
4. Remove or replace the nonfunctional proactive token-refresh scaffold; keep
   reactive browser-profile recovery as the verified path.
5. Decide whether AUrzMb should stay typed API-only or get generated proto
   metric-series messages.
6. Decide whether `video download` should keep the current manual-fallback
   UX or get a real CDN capture and a browser-assisted path.
7. Re-capture `artifact get` against the live service and either verify
   `v9rmvd` or keep the list-scan fallback as the canonical path.
8. Verify `chat config` end-to-end (or hide it until there is a real caller).
9. Capture `izAoDd` only if a real bulk-add CLI caller is introduced.
