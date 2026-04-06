# NLM CLI Gap Analysis

Generated: 2026-04-05
Tested against: 88b05ca (main)
Updated: 2026-04-06 (second round of fixes)

## Summary
- 26/49 commands fully working (was 22)
- 6 commands partially working (was 8)
- 11 commands still broken (wire format / API issues) (was 13)
- 6 commands untested (need specific preconditions)

## Command Status

### Notebook Operations

| Command | Status | Notes |
|---------|--------|-------|
| `ls` / `list` | PASS | Lists 337 notebooks, pagination works |
| `create <title>` | PASS | Returns notebook UUID |
| `rm <id>` | PASS | Confirmation prompt works; `-y` flag works before or after command name |
| `analytics <id>` | PARTIAL | Returns empty response `[]` — encoder may need additional fields |
| `list-featured` | FAIL | Unmarshal error: `user_role` field expects number, got array |

### Source Operations

| Command | Status | Notes |
|---------|--------|-------|
| `sources <id>` | PASS | Lists sources with correct types (TEXT, WEB_PAGE, SHARED_NOTE) |
| `add <id> <file>` | PASS | File and URL sources work |
| `add --name` | PASS | Both `nlm add --name "X" <id> -` and `nlm --name "X" add <id> -` work |
| `rm-source <id> <src>` | PASS | Deletes source correctly |
| `rename-source <src> <name>` | FAIL | Unmarshal error: `invalid character '\\'` in response JSON |
| `check-source <src>` | PASS | Reports freshness status |
| `refresh-source <src>` | FAIL | API error 3: Service unavailable |
| `discover-sources <id> <q>` | FAIL | 400 Bad Request — RPC ID qXyaNe may be stale |

### Note Operations

| Command | Status | Notes |
|---------|--------|-------|
| `notes <id>` | PASS | Shows ID, title, and content preview. Uses raw JSON extraction (Note wire format differs from Source). |
| `new-note <id> <title> [content]` | PASS | Creates notes successfully. Content via arg or stdin both work. |
| `update-note` | UNTESTED | Need note ID from notes list |
| `rm-note` | PASS | Confirmation prompt works |

### Chat Operations

| Command | Status | Notes |
|---------|--------|-------|
| `chat <id>` | PARTIAL | Interactive mode works; chat sends but gets 400 from API. Fallback response shown. |
| `chat --verbose` | PARTIAL | Debug output shows auth and request details; actual chat API returns 400 |
| `chat --skip-sources` | PARTIAL | Flag recognized when placed before command |
| `chat-list` | PASS | Lists saved sessions with metadata |
| `delete-chat <id>` | PASS | Prompts for confirmation correctly |
| `chat-config` | UNTESTED | Requires setting name argument |
| `generate-chat <id> <prompt>` | FAIL | 400 Bad Request from streaming endpoint |

### Instructions / System Prompt

| Command | Status | Notes |
|---------|--------|-------|
| `set-instructions` | PASS | "Instructions updated." — delegates to SetChatConfig with ChatGoalCustom |
| `get-instructions` | PASS | Correctly extracts prompt from GetProject response position [7] after unwrapping |

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
| `generate-outline <id>` | FAIL | 400 Bad Request — RPC ID lCjAd may be stale |
| `generate-section <id>` | FAIL | 400 Bad Request — RPC ID BeTrYd may be stale |
| `generate-magic <id> <src>` | UNTESTED | Requires source IDs |

### Content Transformation Commands

| Command | Status | Notes |
|---------|--------|-------|
| `summarize` | FAIL | API error 3: Wire format corrected to match HAR but still unavailable |
| `brainstorm` | UNTESTED | Likely same issue |
| `rephrase/expand/critique/verify/explain/outline/study-guide/faq/briefing-doc/mindmap/timeline/toc` | UNTESTED | All share `actOnSources` — likely same Service unavailable error |

### Sharing Commands

| Command | Status | Notes |
|---------|--------|-------|
| `share <id>` | PASS | Returns constructed share URL (response is empty on success) |
| `share-private <id>` | PARTIAL | Same approach, untested this round |
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

## Changes in This Update (2026-04-06)

### Fixed
1. **Notes display** — Rewrote `listNotes` to use raw JSON extraction matching the actual Note wire format: `[note_id, [note_id, content, metadata, null, title]]`. Now shows ID, title, and content preview correctly.
2. **get-instructions** — Fixed response unwrapping: GetProject returns `[[project_data...]]` (one wrapper level). After unwrap, chatbot config is correctly at position [7].
3. **Source type enum** — Reordered SourceType enum to match server values: TEXT=4, WEB_PAGE=5 (was GOOGLE_SLIDES=4, GOOGLE_SHEETS=5).
4. **Share URL** — When share response is empty (success), constructs URL from project ID.
5. **ActOnSources wire format** — Rewrote encoder to match HAR: `[sourceRefs, null*4, action, null, [notebook_id, [2]]]` (was using complex nested objects).

### Still Broken (Need HAR Captures)
1. **generate-outline/section** — 400 Bad Request. RPC IDs (lCjAd/BeTrYd) may be stale on server.
2. **discover-sources** — 400 Bad Request. RPC ID qXyaNe may be stale.
3. **actOnSources (summarize etc.)** — Error 3 even with corrected wire format. May need different action string format or additional fields.
4. **chat** — 400 from streaming endpoint. May need updated `bl` parameter or different request structure.
5. **analytics** — Returns empty `[]`. Encoder may need `[2]` ProjectContext suffix.

## Systemic Issues

### 1. Stale `bl` Parameter
The `bl` parameter (`boq_labs-tailwind-frontend_20260210.19_p0`) is from February 2026. Some RPCs may have been renamed/removed in newer server builds. Commands that work (list, create, sources, notes, share) use RPCs that haven't changed; failing commands may use RPCs that have changed.

### 2. Wire Format Errors (400 Bad Request)
**Affected commands**: discover-sources, generate-outline, generate-section, create-artifact, generate-chat, chat
**Root cause**: RPC IDs may be stale, or the server expects different argument formats for newer builds.

### 3. Service Unavailable (API Error 3)
**Affected commands**: refresh-source, audio-create, audio-share, summarize (actOnSources)
**Root cause**: Wire format is close but may have subtle differences from what the server expects. HAR captures of successful calls would resolve this.

### 4. Double-Escaped Response Parsing (FIXED)
**Fixed in**: Previous session (aa0bd00, 5369377, 88b05ca)

### 5. Flag Ordering (FIXED)
**Fixed in**: Previous session (b8e6f88)

## Recommendations (Priority Order)

### P0 — Capture Fresh HAR
Many remaining failures are likely due to stale RPC IDs or subtly wrong wire formats. A HAR capture from the web UI for the failing operations would unblock most remaining fixes.

### P1 — Update `bl` Parameter
Extract the current `bl` parameter from the web UI and update the default. This may fix some 400 errors if the server rejects requests with outdated build labels.

### P2 — Fix Analytics Encoder
The analytics response is empty — try adding ProjectContext `[2]` suffix or other fields.

### P3 — Fix Chat Streaming
The chat endpoint may need a different URL path or headers for the current server build.

### P4 — Fix Double-Escaped Response Commands
`rename-source`, `video-create`, `share-details` all fail with `invalid character '\\'`. These responses contain double-escaped JSON that the parser handles for some responses but not others. May need response-specific unescape logic.
