# NLM CLI Gap Analysis

Generated: 2026-04-05
Tested against: 88b05ca (main)
Updated: 2026-04-05 (final retest after 10 fix commits)

## Summary
- 22/49 commands fully working
- 8 commands partially working
- 13 commands still broken (wire format / API issues)
- 6 commands untested (need specific preconditions)

## Command Status

### Notebook Operations

| Command | Status | Notes |
|---------|--------|-------|
| `ls` / `list` | PASS | Lists 334 notebooks, pagination works |
| `create <title>` | PASS | Returns notebook UUID |
| `rm <id>` | PASS | Confirmation prompt works; `-y` flag works before or after command name |
| `analytics <id>` | PASS | Shows source/note/audio counts |
| `list-featured` | FAIL | Unmarshal error: `user_role` field expects number, got array |

### Source Operations

| Command | Status | Notes |
|---------|--------|-------|
| `sources <id>` | PASS | Lists sources; source types are wrong (URL shows as GOOGLE_SHEETS, text as GOOGLE_SLIDES) |
| `add <id> <file>` | PASS | File and URL sources work |
| `add --name` | PASS | Both `nlm add --name "X" <id> -` and `nlm --name "X" add <id> -` work (flag reordering fix) |
| `rm-source <id> <src>` | PASS | Deletes source correctly |
| `rename-source <src> <name>` | FAIL | Unmarshal error: `invalid character '\\'` in response JSON |
| `check-source <src>` | PASS | Reports freshness status |
| `refresh-source <src>` | FAIL | API error 3: Service unavailable |
| `discover-sources <id> <q>` | FAIL | 400 Bad Request — wire format likely wrong |

### Note Operations

| Command | Status | Notes |
|---------|--------|-------|
| `notes <id>` | PARTIAL | No longer crashes. Shows table but fields mostly empty — Note wire format differs from Source proto. Needs dedicated Note message type. |
| `new-note <id> <title> [content]` | PASS | Creates notes successfully. Content via arg or stdin both work. |
| `update-note` | UNTESTED | Blocked by notes list failure (can't get note ID) |
| `rm-note` | UNTESTED | Same blocker |

### Chat Operations

| Command | Status | Notes |
|---------|--------|-------|
| `chat <id>` | PARTIAL | Interactive mode works; chat sends but gets 400 from API. Fallback "Hello" response shown. History/save/multiline commands work locally. |
| `chat --verbose` | PARTIAL | Debug output shows auth and request details; actual chat API returns 400 |
| `chat --skip-sources` | PARTIAL | Flag recognized when placed before command |
| `chat-list` | PASS | Lists 6 saved sessions with metadata |
| `delete-chat <id>` | PASS | Prompts for confirmation correctly |
| `chat-config` | UNTESTED | Requires setting name argument |
| `generate-chat <id> <prompt>` | FAIL | 400 Bad Request from streaming endpoint |

### Instructions / System Prompt

| Command | Status | Notes |
|---------|--------|-------|
| `set-instructions` | PASS | "Instructions updated." — delegates to SetChatConfig with ChatGoalCustom |
| `get-instructions` | PARTIAL | Returns "No custom instructions set." — response position [7] extraction may need adjustment |

### Audio Operations

| Command | Status | Notes |
|---------|--------|-------|
| `audio-list <id>` | PASS | Shows audio overviews with status |
| `audio-create <id> <instr>` | FAIL | API error 3: Service unavailable |
| `audio-get <id>` | PASS | Returns audio overview (title/ID empty but ready=true) |
| `audio-download <id> [file]` | UNTESTED | Requires `--direct-rpc` flag |
| `audio-rm <id>` | UNTESTED | No audio to delete |
| `audio-share <id>` | FAIL | API error 3: Service unavailable |

### Video Operations

| Command | Status | Notes |
|---------|--------|-------|
| `video-list <id>` | PASS | Returns empty list correctly |
| `video-create <id> <instr>` | FAIL | Response parse error: `invalid character '\\'` |
| `video-download <id> [file]` | UNTESTED | No video to download |

### Artifact Operations

| Command | Status | Notes |
|---------|--------|-------|
| `artifacts` / `list-artifacts` | PASS | Returns empty list correctly |
| `create-artifact <id> <type>` | FAIL | 400 Bad Request — wire format wrong |
| `get-artifact <id>` | UNTESTED | No artifacts to fetch |
| `rename-artifact` | UNTESTED | No artifacts |
| `delete-artifact` | UNTESTED | No artifacts |

### Generation Commands

| Command | Status | Notes |
|---------|--------|-------|
| `generate-guide <id>` | PASS | Works with ProjectContext [2] suffix + chunked parser fix |
| `generate-outline <id>` | FAIL | 400 Bad Request |
| `generate-section <id>` | FAIL | 400 Bad Request |
| `generate-magic <id> <src>` | UNTESTED | Requires source IDs |

### Content Transformation Commands

| Command | Status | Notes |
|---------|--------|-------|
| `summarize` | FAIL | API error 3: Service unavailable |
| `brainstorm` | UNTESTED | Likely same issue |
| `rephrase/expand/critique/verify/explain/outline/study-guide/faq/briefing-doc/mindmap/timeline/toc` | UNTESTED | All share `actOnSources` — likely same Service unavailable error |

### Sharing Commands

| Command | Status | Notes |
|---------|--------|-------|
| `share <id>` | PARTIAL | Returns success but "URL format not recognized" |
| `share-private <id>` | PARTIAL | Same — "URL format not recognized" |
| `share-details <id>` | FAIL | Parse error: `invalid character '\\'` |

### Other Commands

| Command | Status | Notes |
|---------|--------|-------|
| `auth` | PASS | Browser auth works with `--cdp-url`; profile copy fails (redirects to sign-in) |
| `refresh` | PASS | Credentials refreshed successfully |
| `feedback <msg>` | PASS | Submits feedback |
| `hb` | PASS | Heartbeat succeeds silently |
| `--help` | PASS | Comprehensive command listing |
| `research` | UNTESTED | Registered (5fa21a9). Wire format guessed — needs live verification. |

### Edge Cases

| Test | Status | Notes |
|------|--------|-------|
| Missing args (all commands) | PASS | All show usage text, no panics |
| Empty notebook: sources | PASS | Empty table headers |
| Empty notebook: notes | PASS | Empty table headers |
| Empty notebook: rm | PASS | Deletes cleanly |
| Flag ordering | FIXED | `reorderArgs()` moves flags before command: both `nlm rm -y <id>` and `nlm -y rm <id>` work |

## Systemic Issues

### 1. Double-Escaped JSON Response Parsing (FIXED)
**Fixed in**: aa0bd00 (unescape), 5369377 (chunked parser rewrite), 88b05ca (beprotojson coercion)
**Root cause**: The chunked response parser used line-based scanning with size counting, but the batchexecute format's size prefix doesn't correspond to simple byte counts. This caused cross-chunk data leaking. Fixed by rewriting the chunked parser to use bracket-based JSON extraction (findJSONEnd). Also added string-to-number coercion in beprotojson and graceful skipping of unparseable repeated items.

### 2. Wire Format Errors (400 Bad Request)
**Affected commands**: discover-sources, generate-outline, generate-section, create-artifact, generate-chat
**Root cause**: The argument encoders for these RPCs are sending malformed payloads. Many have placeholder `[]interface{}{}` or missing `ProjectContext` suffixes.

### 3. Service Unavailable (API Error 3)
**Affected commands**: refresh-source, audio-create, audio-share, summarize (actOnSources)
**Root cause**: These RPCs may require specific preconditions (e.g., sources must be fully ingested), or the wire format is subtly wrong causing the server to reject.

### 4. Source Type Enum Mapping
URLs display as `SOURCE_TYPE_GOOGLE_SHEETS`, text as `SOURCE_TYPE_GOOGLE_SLIDES`. The proto enum values don't match actual server-side types. Need to capture real type values from HAR and update the enum mapping.

### 5. Flag Ordering (FIXED)
**Fixed in**: b8e6f88
Added `reorderArgs()` that moves flags to before the command name before calling `flag.Parse()`. Both `nlm rm -y <id>` and `nlm -y rm <id>` now work. Handles boolean vs string flags, bare `-` (stdin), and `--` (end of flags).

## Feature Gaps vs Web UI

### Implemented and Working
- Notebook CRUD (list, create, delete)
- Source add (file, URL, stdin with `--name`)
- Source delete, check freshness
- Interactive chat (session management, history, multiline)
- Audio list, get overview
- Artifact listing
- Chat session management (list, save, history)
- Feedback submission
- Auth via CDP URL

### Partially Implemented
- Chat (interactive mode works, but API calls return 400)
- Sharing (succeeds but URL extraction broken)
- Notes (create may work server-side but response parsing fails)
- Source rename (sends request but response parsing fails)

### Missing from CLI
- `set-instructions` / `get-instructions` commands (not registered)
- `research` / deep research command (not registered)
- Artifact content viewing/export
- Flashcards, slides, report generation wrappers
- WebRTC interactive audio
- Real-time chat streaming with thinking traces rendered live
- Source type display (enum mapping wrong)
- Chat response length / source selection config

## Wire Format Issues Found
1. **Double-escaped JSON**: Most critical — responses from batchexecute contain escaped JSON strings that aren't being unescaped before protobuf unmarshaling
2. **ActOnSources**: API error 3 suggests wire format issue with source ID nesting or action type encoding
3. **CreateArtifact (R7cb6c)**: Missing payload matrix for artifact type selection
4. **GenerateOutline/Section**: Missing required ProjectContext suffix arrays
5. **DiscoverSources**: Wire format not validated against HAR capture
6. **Chat streaming**: Using wrong protocol — should be gRPC-Web, not batchexecute

## Recommendations (Priority Order)

### P0 — Fix Response Parsing (DONE - aa0bd00)
Fixed double-escaped JSON handling via `unescapeResponseData()` in batchexecute.

### P1 — Register Missing Commands (DONE - 9ca97af, 5fa21a9)
Registered `set-instructions`, `get-instructions`, and `research` in CLI dispatch.

### P1 — Fix Wire Formats for 400 Errors (PARTIALLY DONE - 21887bb)
Added ProjectContext [2] suffix to GenerateOutline, GenerateSection, GenerateNotebookGuide, DiscoverSources, GetNotes. Fixed ActOnSources 3-level source nesting. CreateArtifact still needs HAR capture for R7cb6c type config matrix.

### P2 — Fix Source Type Enum Mapping
Capture source type values from HAR and update the proto enum to match server-side values.

### P2 — Fix Chat API Calls
The chat 400 errors suggest the gRPC-Web request format needs updating. Verify source ID nesting and required headers match current server expectations.

### P2 — Improve Flag Ordering UX
Either switch to a subcommand-aware parser or add a pre-parse step that extracts known flags from anywhere in argv before passing to `flag.Parse()`.

### P3 — Fix ActOnSources Wire Format
Capture HAR for summarize/brainstorm/etc. to verify source ID nesting format and action type encoding.

### P3 — Add Artifact Viewing/Export
Parse artifact content from `ListArtifacts` response. Add `artifact-view` and `artifact-export` commands.

### P4 — Deep Research Polling Engine
Implement background polling loop for `StartDeepResearch` / `PollDeepResearch` with progressive output.

### P5 — WebRTC Interactive Audio
Requires full WebRTC stack in Go — significant undertaking. Consider as a stretch goal.
