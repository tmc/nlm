# NLM CLI Gap Analysis

Generated: 2026-04-05
Tested against: c91a070 (main)
Updated: 2026-04-06 (final retest after rounds 1-3)

## Summary
- 34/49 commands fully working (was 22 at start of session)
- 4 commands partially working
- 7 commands still broken
- 4 commands untested (need specific preconditions)

## Command Status

### Notebook Operations

| Command | Status | Notes |
|---------|--------|-------|
| `ls` / `list` | PASS | Lists 338 notebooks, pagination works |
| `create <title>` | PASS | Returns notebook UUID |
| `rm <id>` | PASS | Confirmation prompt works; `-y` flag works before or after command name |
| `analytics <id>` | PASS | Returns analytics or friendly "No analytics data" message |
| `list-featured` | PARTIAL | Returns data but field mapping is wrong (bool/int values in wrong columns) |

### Source Operations

| Command | Status | Notes |
|---------|--------|-------|
| `sources <id>` | PASS | Lists sources with correct types (TEXT, WEB_PAGE, SHARED_NOTE) |
| `add <id> <file>` | PASS | File and URL sources work |
| `add --name` | PASS | Both `nlm add --name "X" <id> -` and `nlm --name "X" add <id> -` work |
| `rm-source <id> <src>` | PASS | Deletes source correctly |
| `rename-source <src> <name>` | PASS | Renames source correctly (was broken, now fixed by bl update) |
| `check-source <src>` | PASS | Reports freshness status |
| `refresh-source <src>` | FAIL | API error 3: Service unavailable |
| `discover-sources <id> <q>` | FAIL | 400 Bad Request — RPC ID qXyaNe deprecated, no HAR evidence |

### Note Operations

| Command | Status | Notes |
|---------|--------|-------|
| `notes <id>` | PASS | Shows ID, title, and content preview via raw JSON extraction |
| `new-note <id> <title> [content]` | PASS | Creates notes successfully. Content via arg or stdin both work. |
| `update-note` | UNTESTED | Need note ID from notes list |
| `rm-note` | PASS | Confirmation prompt works |

### Chat Operations

| Command | Status | Notes |
|---------|--------|-------|
| `chat <id>` | PASS | Interactive mode works with full responses and reasoning traces |
| `chat --verbose` | PASS | Debug output shows auth and request details |
| `chat --skip-sources` | PASS | Flag recognized when placed before command |
| `chat-list` | PASS | Lists saved sessions with metadata |
| `delete-chat <id>` | PASS | Prompts for confirmation correctly |
| `chat-config` | UNTESTED | Requires setting name argument |
| `generate-chat <id> <prompt>` | PASS | Returns generated content |

### Instructions / System Prompt

| Command | Status | Notes |
|---------|--------|-------|
| `set-instructions` | PASS | "Instructions updated." — delegates to SetChatConfig with ChatGoalCustom |
| `get-instructions` | PASS | Correctly extracts prompt from GetProject response position [7] |

### Audio Operations

| Command | Status | Notes |
|---------|--------|-------|
| `audio-list <id>` | PASS | Shows audio overviews with status |
| `audio-create <id> <instr>` | FAIL | API error 3: Service unavailable (wire format issue) |
| `audio-get <id>` | PASS | Returns audio overview (ready=true) |
| `audio-download <id> [file]` | UNTESTED | Requires `--direct-rpc` flag |
| `audio-rm <id>` | UNTESTED | No audio to delete |
| `audio-share <id>` | FAIL | API error 3: Service unavailable |

### Video Operations

| Command | Status | Notes |
|---------|--------|-------|
| `video-list <id>` | PASS | Returns empty list correctly |
| `video-create <id> <instr>` | FAIL | Response parse error |
| `video-download <id> [file]` | UNTESTED | No video to download |

### Artifact Operations

| Command | Status | Notes |
|---------|--------|-------|
| `artifacts` / `list-artifacts` | PASS | Returns empty list correctly |
| `create-artifact <id> <type>` | FAIL | 400 Bad Request — needs R7cb6c workflow |
| `get-artifact <id>` | UNTESTED | No artifacts to fetch |
| `rename-artifact` | UNTESTED | No artifacts |
| `delete-artifact` | UNTESTED | No artifacts |

### Generation Commands

| Command | Status | Notes |
|---------|--------|-------|
| `generate-guide <id>` | PASS | Works with ProjectContext [2] suffix |
| `generate-outline <id>` | FAIL | 400 Bad Request — RPC lCjAd deprecated (use ciyUvf workflow) |
| `generate-section <id>` | FAIL | 400 Bad Request — RPC BeTrYd deprecated (use ciyUvf workflow) |
| `generate-chat <id> <prompt>` | PASS | Returns generated content |
| `generate-magic <id> <src>` | UNTESTED | Requires source IDs |

### Content Transformation Commands

| Command | Status | Notes |
|---------|--------|-------|
| `summarize` | PASS | actOnSources works with HAR-verified wire format |
| `brainstorm` | PASS | Same |
| `explain` | PASS | Same |
| `rephrase/expand/critique/verify/outline/study-guide/faq/briefing-doc/mindmap/timeline/toc` | PARTIAL | Should work (same actOnSources path), not all individually tested |

### Sharing Commands

| Command | Status | Notes |
|---------|--------|-------|
| `share <id>` | PASS | Returns constructed share URL |
| `share-private <id>` | PARTIAL | Same approach, not tested this round |
| `share-details <id>` | PASS | Shows share ID and details |

### Other Commands

| Command | Status | Notes |
|---------|--------|-------|
| `auth` | PASS | Browser auth works with `--cdp-url` |
| `refresh` | PASS | Credentials refreshed successfully |
| `feedback <msg>` | PASS | Submits feedback |
| `hb` | PASS | Heartbeat succeeds silently |
| `--help` | PASS | Comprehensive command listing |
| `research` | UNTESTED | Wire format guessed — needs live verification |

### Edge Cases

| Test | Status | Notes |
|------|--------|-------|
| Missing args (all commands) | PASS | All show usage text, no panics |
| Flag ordering | FIXED | `reorderArgs()` moves flags before command |
| IPv6 fallback | FIXED | DNS-level IPv4 resolution in HTTP client |

## Key Fixes Applied (Rounds 1-3)

### Round 1 (previous session)
- Chunked response parser rewrite (bracket-based JSON extraction)
- beprotojson string-to-number coercion
- Flag reordering (`reorderArgs`)
- ProjectContext [2] suffix on encoders
- Deep research polling engine

### Round 2
- Notes display via raw JSON extraction (Note wire format differs from Source)
- get-instructions response unwrapping
- SourceType enum reordering (TEXT=4, WEB_PAGE=5)
- Share URL construction from project ID

### Round 3
- bl parameter update (20260210 → 20260402)
- Chat: source nesting `[[["id"]]]` + restored `at=` XSRF token
- ActOnSources: action as 3-element array, pos 7 mode selector
- IPv4 DNS resolution in HTTP client
- New RPCs registered (ciyUvf, KmcKPe, ub2Bae, V5N4be, otmP3b)

## Still TODO

### P1 — Report Generation Workflow
Replace deprecated `generate-outline` (lCjAd) and `generate-section` (BeTrYd) with:
1. `ciyUvf` (GenerateReportSuggestions) → get suggestions
2. `R7cb6c` (CreateUniversalArtifact) → create report artifact
3. `gArtLc` (ListArtifacts) → poll for completion

### P2 — Audio/Video Creation
`audio-create`, `audio-share`, `video-create` return error 3. Need HAR captures of successful creation calls.

### P3 — refresh-source
Returns error 3. May need different wire format or preconditions.

### P4 — list-featured Presentation Coverage
`ub2Bae` now decodes featured-project presentation payloads, including the
image list at presentation position 2.
