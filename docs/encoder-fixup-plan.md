# Encoder Fixup Plan

## Background

Commit `78706bd` (gen: regenerate method encoders and service clients)
replaced HAR-verified, hand-coded RPC encoders with generic
`argbuilder.EncodeRPCArgs` templates. The argbuilder can only express
flat field references (`%field_name%`); it cannot express literal arrays
(`[2]`), null padding, nesting levels, or computed values. Many RPCs
require these structures, so the regeneration introduced wire-format
regressions that the server rejects with gRPC error code 3 (UNAVAILABLE).

Chat (gRPC-Web endpoint) and a few RPCs whose encoders were kept
hand-coded (CreateAudioOverview, DeleteSources, MutateSource,
RenameArtifact) still work.

### Diagnosis method

Each broken encoder can be confirmed by running the command with
`--debug` and inspecting the `f.req` payload. The server returns
`[3]` (UNAVAILABLE) in the error slot when it cannot route or decode
the arguments.

---

## Phase 1: Critical encoder restores (5 RPCs)

These encoders are confirmed broken. Restore each to the last known
working version (referenced by commit hash) and add a `// Wire format
verified against HAR capture — do not regenerate.` guard comment.

### 1.1 CreateNote (CYK0Xb)

**File:** `gen/method/LabsTailwindOrchestrationService_CreateNote_encoder.go`
**Last working commit:** `247c09b`
**Symptom:** `nlm new-note` → error 3

Correct wire format (7 positions):
```
[projectId, htmlContent, [1], null, title, null, [2]]
```

Restore:
- Position 0: project ID
- Position 1: content (wrap plain text in `<p>` tags)
- Position 2: `[1]` (note type)
- Position 3: `nil`
- Position 4: title
- Position 5: `nil`
- Position 6: `[2]` (ProjectContext)

### 1.2 ActOnSources (yyryJe)

**File:** `gen/method/LabsTailwindOrchestrationService_ActOnSources_encoder.go`
**Last working commit:** `4022605`
**Symptom:** `nlm summarize`, `nlm rephrase`, etc. → error 3

Correct wire format (8 positions):
```
[sourceRefs, null, null, null, null, actionConfig, null, modeSelector]
```

Where:
- `sourceRefs` = `[[["source-id-1"]], [["source-id-2"]]]` (2-level nesting per source)
- `actionConfig` = `["summarize", [["[CONTEXT]", ""]], ""]`
- `modeSelector` = `[2, null, [1], [1]]`

Note: project_id is passed via URL source-path, NOT in args.

### 1.3 GenerateReportSuggestions (ciyUvf)

**File:** `gen/method/LabsTailwindOrchestrationService_GenerateReportSuggestions_encoder.go`
**Last working commit:** `0576e13`
**Symptom:** `nlm generate-outline` → error 3

Correct wire format (3 positions):
```
[projectContext, projectId, sourceRefs]
```

Where:
- `projectContext` = `[2, nil, nil, [1, nil, nil, nil, nil, nil, nil, nil, nil, nil, [1]], [[1, 4, 2, 3, 6, 5]]]`
- `sourceRefs` = `[["source-id-1"], ["source-id-2"]]` (1-level wrapping per source)

### 1.4 MutateNote (cYAfTb)

**File:** `gen/method/LabsTailwindOrchestrationService_MutateNote_encoder.go`
**Last working commit:** `d4017b8`
**Symptom:** `nlm update-note` → error 3

Correct wire format:
```
[projectId, noteId, [[[htmlContent, title, [], 0]]], [2]]
```

Where content gets `<p>` wrapping for plain text.

### 1.5 ShareProject (QDyure)

**File:** `gen/method/LabsTailwindOrchestrationService_ShareProject_encoder.go`
**Last working commit:** `0576e13`
**Symptom:** `nlm share` → error 3

Correct wire format:
```
[[[projectId, null, [1, accessLevel], [0, ""]]], 1, null, [2]]
```

Where `accessLevel` is computed from the request's settings field.

---

## Phase 2: Important encoder restores (5 RPCs)

These encoders lost structural elements (ProjectContext `[2]` or `[4]`
suffixes, position padding, default values) that may cause failures
depending on server-side validation strictness. Test each and fix if
broken.

### 2.1 SubmitFeedback (uNyJKe)

**Last working commit:** `c36c784`

Correct wire format is 9 positions with complex context fields. Low
usage priority but should be restored for correctness.

### 2.2 GenerateFreeFormStreamed

**File:** `gen/method/LabsTailwindOrchestrationService_GenerateFreeFormStreamed_encoder.go`

This file is **EMPTY** — the encoder function is missing entirely.
Used by `generate-chat`. Must implement — check commit history before
`78706bd` for the working version.

### 2.3 CheckSourceFreshness

**Last working commit:** `144aed0`

Was: `[null, ["source-id"], [4]]`
Now: `[%source_id%]`

Missing null prefix and ProjectContext `[4]`.

### 2.4 RefreshSource

**Last working commit:** `144aed0`

Was: `[null, ["source-id"], [4]]`
Now: `[%source_id%]`

Same structural loss as CheckSourceFreshness.

### 2.5 DeleteNotes (AH0mwd)

**Last working commit:** `d4017b8`

Was: `[null, null, [noteIds], [2]]` (project_id via source-path)
Now: `[%project_id%, null, %note_ids%, [2]]`

The project_id injection may cause issues since the hand-coded version
omitted it (passed via URL instead).

---

## Phase 3: Minor encoder fixes (5 RPCs)

Lower risk — the simplified format may still work. Test first, fix
only if broken.

### 3.1 GetNotes

Was: timestamp-based `[project_id, null, [timestamp_sec, timestamp_nano], [2]]`
Now: `[%project_id%, null, %note_ids%, [2]]`

Semantics changed from "get notes since timestamp" to "get specific
note IDs" — verify which the server expects.

### 3.2 GetConversations

Lost default `limit=20` when no limit provided. May return empty or
error without it.

### 3.3 DeleteChatHistory

Was: `[null, null, projectId]` — the template shows `[null, null, %project_id%]`
which may actually be equivalent. Verify.

### 3.4 GenerateNotebookGuide / GenerateOutline / GenerateSection

All lost ProjectContext `[2]` suffix. Currently `[%project_id%]`,
should be `[project_id, [2]]`. Low risk since `generate-guide`
**passed testing** — possibly the server accepts both formats for
these RPCs.

### 3.5 DiscoverSources

Was: `[project_id, query, [2]]`
Now: `[%project_id%, %query%]`

Lost ProjectContext `[2]`. Low risk.

---

## Phase 4: Prevent future regressions

### 4.1 Add `// wire-format: hand-coded` guard comments

Every encoder that has a verified wire format should have a header:
```go
// Wire format verified against HAR capture — do not regenerate.
// See docs/encoder-fixup-plan.md for the expected format.
```

The codegen template should skip files with this marker.

### 4.2 Update proto arg_format annotations

The `arg_format` annotations in `orchestration.proto` are misleading
since they describe simplified formats that don't match reality. Either:

- (a) Remove them for hand-coded encoders, or
- (b) Extend the argbuilder DSL to support `null`, literal arrays,
  nesting operators, and computed fields

Option (a) is simpler and matches the current reality. The argbuilder
works for truly simple RPCs (ListProjects, Heartbeat, etc.) but cannot
express the batchexecute wire quirks.

### 4.3 Add encoder integration tests

For each restored encoder, add a test that:
1. Constructs a sample request
2. Calls the encoder
3. Asserts the output matches the expected wire format (JSON comparison)

This prevents future regeneration from silently breaking formats.

---

## Execution order

1. **Phase 1** — restore the 5 critical encoders. This unblocks:
   - `new-note`, `update-note` (CreateNote, MutateNote)
   - `summarize`, `rephrase`, `expand`, `critique`, `brainstorm`,
     `verify`, `explain`, `outline`, `study-guide`, `faq`,
     `briefing-doc`, `mindmap`, `timeline`, `toc` (ActOnSources)
   - `generate-outline` (GenerateReportSuggestions)
   - `share` (ShareProject)

2. **Phase 2** — restore important encoders and verify edge cases.

3. **Phase 3** — test and fix minor encoders if needed.

4. **Phase 4** — add guards and tests to prevent regressions.

## Files modified

Phase 1 touches only `gen/method/` encoder files. No changes to
proto definitions, argbuilder, client code, or CLI code.

Phase 4 optionally touches `proto/notebooklm/v1alpha1/orchestration.proto`
(annotation cleanup) and adds test files.
