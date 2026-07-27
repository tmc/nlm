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
- Authentication-shaped failures trigger one silent credential re-harvest from
  the exact cached browser profile and then retry the command. Set
  `NLM_AUTO_REFRESH=false` to disable this recovery.
- `nlm chat` is a streaming interactive client with persistent on-disk
  sessions, history, and slash commands.
- Chat and generation commands select sources by UUID, title regular
  expression, and server-side label.
- `nlm research --md` emits the deep-research Markdown report and rewrites its
  citation markers as URL-backed Markdown footnotes.
- `create_note` and `add_source_text` provide MCP context injection.
  `start_deep_research`, `poll_deep_research`, and `watch_deep_research`
  provide asynchronous and blocking research control; the watch tool emits MCP
  progress notifications when the caller supplies a progress token.
- Directory-tree sync, RichDocument note rendering, and txtar citation
  `file:line` resolution are implemented.

The authentication constraint is narrower: browser login harvests credentials
from a profile already signed into Google. It does not provide unattended SSO
on a fresh machine.

## Confirmed Product Gaps

### 1. Broad artifact export

`artifact export` fetches any READY artifact that exposes a server-rendered
download and emits Google-AI-mode type-4 flashcard apps as Markdown, JSON, TSV,
or their self-contained HTML. It does not synthesize semantic exports for
native type-9 flashcards, quizzes, data tables, or mind maps.

Native type-9 flashcard content and mind-map structure are not sufficiently
modeled to invent exporters. Any work on those formats starts with a real
capture and a lossless corpus gate.

Status: high priority, capture-blocked for successful native type-9 card bodies
and mind-map structure. The capture-proven type-4 app path is implemented
separately and must not be generalized to type 9.

### 2. Agent onboarding and competitive breadth

The in-repo skill is concise and delegates to current `--help`, but there is no
one-shot `nlm --ai` documentation dump or skill installer. The MCP surface also
omits HTTP/SSE transport, in-MCP authentication recovery, local file-path
injection, and some sharing, batch, and cross-notebook operations exposed by
broader competitors. AI-assisted auto-labeling is not implemented.

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

### 4. `chat config` server semantics unverified

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

1. Capture and lossless-verify a successful native type-9 flashcard or quiz
   payload before modeling its card content.
2. Decide whether AUrzMb should stay typed API-only or get generated proto
   metric-series messages.
3. Decide whether `video download` should keep the current manual-fallback
   UX or get a real CDN capture and a browser-assisted path.
4. Verify `chat config` end-to-end (or hide it until there is a real caller).
5. Capture `izAoDd` only if a real bulk-add CLI caller is introduced.
