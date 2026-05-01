---
title: Remaining Gaps and Agent Implementation Plan
date: 2026-04-14
---

# nlm Remaining Gaps

Prioritized list of gaps identified by NotebookLM self-analysis after the R7cb6c wire format fixes, beprotojson.Marshal improvements, and HAR fixture tests.

## High Impact

### 1. Broken argbuilder Encoders

Many RPC encoders use the generic `argbuilder.EncodeRPCArgs` with flat formats like `[%project_id%]`, missing null padding and ProjectContext suffix that Google's batchexecute endpoints require. These commands will return gRPC error 3 (UNAVAILABLE).

**Affected files and commands:**

| Command | Encoder File | RPC ID | Current Format |
|---------|-------------|--------|----------------|
| `guidebook-publish` | `gen/method/LabsTailwindGuidebooksService_PublishGuidebook_encoder.go` | `khqZz` | `[%guidebook_id%, %settings%]` |
| `guidebook-share` | `gen/method/LabsTailwindGuidebooksService_ShareGuidebook_encoder.go` | `sqTeoe` | flat |
| `guidebook-rm` | `gen/method/LabsTailwindGuidebooksService_DeleteGuidebook_encoder.go` | `ozz5Z` | flat |
| `guidebook-ask` | `gen/method/LabsTailwindGuidebooksService_GenerateAnswer_encoder.go` | `eyWvXc` | flat |
| `delete-artifact` | `gen/method/LabsTailwindOrchestrationService_DeleteArtifact_encoder.go` | varies | `[%artifact_id%]` |
| `audio-rm` | via `EncodeDeleteAudioOverviewArgs` | `hizoJc` | `[%project_id%]` |
| `audio-share` | via `EncodeShareAudioArgs` | `hPTbtc` | `[%share_options%, %project_id%]` |
| `analytics` | via `EncodeGetProjectAnalyticsArgs` | `cFji9` | `[%project_id%]` |

**Fix approach:** Capture HAR samples for each RPC, extract the correct positional array format, and replace the argbuilder calls with hand-verified encoders (or use beprotojson.Marshal once wire-format protos are defined). Mark each with `// Wire format verified against HAR capture`.

### 2. Dead/Deprecated CLI Commands

These commands hit deprecated endpoints that return 400 errors:

| Command | RPC ID | Issue |
|---------|--------|-------|
| `create-artifact` (generic) | `xpWGLf` | Deprecated; should route through R7cb6c |
| `generate-outline` | `lCjAd` | Deprecated endpoint |
| `generate-section` | `BeTrYd` | Deprecated endpoint |

**Fix approach:** Remove or migrate these. `create-artifact` should be refactored to dispatch to the type-specific `create-audio`/`create-video`/`create-slides` code paths via R7cb6c. `generate-outline` and `generate-section` should be replaced with the `ciyUvf` (GenerateReportSuggestions) -> R7cb6c pipeline, or removed if the functionality is redundant with other generation commands.

### 3. MCP Server Missing Capabilities

`internal/nlmmcp/tools.go` does not expose the newly fixed artifact creation or several other high-value capabilities:

| Missing MCP Tool | CLI Equivalent | Priority |
|-----------------|----------------|----------|
| `create_audio_overview` | `create-audio` | High |
| `create_video_overview` | `create-video` | High |
| `create_slide_deck` | `create-slides` | High |
| `start_deep_research` | `research` | High |
| `set_instructions` | `set-instructions` | Medium |
| `get_instructions` | `get-instructions` | Medium |
| `list_artifacts` | `artifacts` | Medium |
| `read_note` | `read-note` | Medium |

**Fix approach:** Add MCP tool definitions in `internal/nlmmcp/tools.go` and handlers that call the existing `api.Client` methods. Follow the pattern of existing tools (e.g., `generate_chat`, `create_notebook`).

## Medium Impact

### 4. Response Parsing Issues

| Command | RPC | Issue |
|---------|-----|-------|
| `list-featured` | `ub2Bae` | Bool/int field misalignment in beprotojson mapping |
| `share-details` | `JFMDGd` | Incomplete parsing, returns empty data |

**Fix approach:** Use `--debug` to dump raw responses, compare field positions against proto definitions, fix field numbers in proto or add custom unmarshaling.

### 5. Media Download Broken

`audio-download` and `video-download` (both require `--direct-rpc`) have response parsing that returns 0 elements. The CDN URL extraction from the deep protobuf structure needs to be verified against live payloads.

**Fix approach:** Capture HAR for audio-download and video-download operations, identify the array position holding the CDN URL, fix the extraction logic in `internal/notebooklm/api/client.go`.

## Low Impact

### 6. Unexposed Proto Functionality

| Proto Message | RPC | Description |
|--------------|-----|-------------|
| `StartDraftRequest` | `exXvGf` | Generative writing workflow |
| `StartSectionRequest` | `pGC7gf` | Section generation |
| `GenerateMagicViewRequest` | varies | Magic view from sources (CLI has command but untested) |

**Fix approach:** Low priority. Wire up if/when HAR captures become available, or if users request these features.

---

## Agent Team Implementation Prompt

Use the following prompt to launch an agent team that addresses all high and medium impact gaps:

```
Launch an agent team to fix the remaining gaps in the nlm codebase. The work is divided into 4 parallel workstreams. Each agent works in an isolated worktree.

### Agent 1: Fix Broken Encoders (worktree)

Read docs/dev/remaining-gaps.md for context. The argbuilder-based encoders for guidebook operations, artifact deletion, audio deletion/sharing, and analytics use flat formats that are missing null padding. These need to be fixed.

For each broken encoder listed in the "Broken argbuilder Encoders" section:

1. Read the current encoder in gen/method/ to understand the RPC ID and current format
2. Read the corresponding service client in gen/service/ to understand how it's called
3. Check if HAR samples exist in /Users/tmc/go/src/github.com/tmc/misc/chrome-to-har/logs/nlm-capture/sources/notebooklm.google.com/_rpc-samples/ for the RPC ID
4. If HAR samples exist, extract the correct wire format and rewrite the encoder as a hand-coded function in a new file gen/method/labs_tailwind_custom_encoders.go (following the pattern in labs_tailwind_overview_custom.go)
5. If no HAR samples exist, add the ProjectContext suffix pattern: the most common fix is changing [project_id] to [nil, nil, project_id, [2]] or similar. Look at working encoders for the same service to infer the pattern.
6. Add fixture-based tests for each fixed encoder

Run go test ./gen/method/... and go build ./... to verify.

### Agent 2: Fix MCP Server (worktree)

Read docs/dev/remaining-gaps.md for context. The MCP server in internal/nlmmcp/tools.go is missing several high-value capabilities.

1. Read internal/nlmmcp/tools.go to understand the existing tool registration pattern
2. Read internal/nlmmcp/handler.go to understand how tools dispatch to api.Client methods
3. Add these MCP tools following the existing patterns:
   - create_audio_overview (calls api.Client.CreateAudioOverview)
   - create_video_overview (calls api.Client.CreateVideoOverview)  
   - create_slide_deck (calls api.Client.CreateSlideDeck)
   - list_artifacts (calls api.Client.ListArtifacts)
   - read_note (calls api.Client.GetNotes + filter)
   - set_instructions / get_instructions (calls api.Client.SetInstructions / GetInstructions)
4. For deep research, read cmd/nlm/main.go to understand the research polling loop, then add a start_deep_research tool that initiates research (the polling can be done by the AI agent calling the tool repeatedly)

Run go test ./internal/nlmmcp/... and go build ./... to verify.

### Agent 3: Remove Dead Commands + Fix Media Download (worktree)

Read docs/dev/remaining-gaps.md for context.

Part A — Dead commands:
1. In cmd/nlm/main.go, find the create-artifact generic command (uses xpWGLf RPC)
2. Remove it from the command dispatch and help text — users should use create-audio/create-video/create-slides instead
3. Find generate-outline and generate-section commands
4. Either remove them or rewire to use working RPCs. Check if generate-guide covers the same use case — if so, just remove with a helpful error message pointing to generate-guide.
5. Update help text and validCommands list

Part B — Media download:
1. Read the audio-download and video-download handlers in cmd/nlm/main.go and internal/notebooklm/api/client.go
2. Run nlm --debug --direct-rpc audio-download <notebook-id> to see the raw response
3. Fix the CDN URL extraction based on what the response actually contains
4. If audio-download/video-download can't be fixed without HAR captures, add clear error messages about what's wrong

Run go test ./cmd/nlm/... and go build ./... to verify.

### Agent 4: Fix Response Parsing (worktree)

Read docs/dev/remaining-gaps.md for context.

1. Run nlm --debug list-featured to see the raw response for the ub2Bae RPC
2. Compare the array positions against the FeaturedProject proto definition in gen/notebooklm/v1alpha1/
3. Fix any field number misalignment in the proto or add custom unmarshaling
4. Run nlm --debug share-details <share-id> (you may need to first run nlm share <notebook-id> to get a share ID)
5. Fix the JFMDGd response parsing

Run go test ./... and go build ./... to verify.

IMPORTANT for all agents:
- Use notebook 00000000-0000-4000-8000-000000000003 for any live API testing
- Do NOT modify proto files (they require buf generate to regenerate)
- Follow the patterns in existing code (Russ Cox style — minimal, clear, no panics)
- Run go vet ./... before finishing
- Stage changes but do not commit
```

This prompt can be used with:
```
# In the nlm project directory:
# Launch all 4 agents in parallel with isolated worktrees
```

Each agent gets its own worktree so changes don't conflict. After all complete, review and merge the changes, resolve any conflicts, then run the full test suite.
