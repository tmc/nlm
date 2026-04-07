# Spec: Clean Output Mode for generate-chat

> **For Claude:** After writing this spec, discover and read the explore phase skill via cache lookup for `skills/dev-explore/SKILL.md`.

## Problem

`nlm generate-chat` outputs raw chunked HTTP-style data with nested JSON arrays (wrb.fr payloads) and escaped strings. The actual answer text is buried and accumulates with each streaming chunk. This makes it unusable for programmatic consumers like Claude Code agents (librarian workflow).

## Requirements

| ID | Requirement | Scope |
|----|-------------|-------|
| FMT-01 | Add `--format` flag to `generate-chat` with values `stream` (default, existing behavior) and `plain` | v1 |
| FMT-02 | When `--format plain`, buffer the full streaming response and print only the final plain text to stdout | v1 |
| FMT-03 | Output must contain no JSON artifacts, chunk markers, wrb.fr payloads, or escape sequences | v1 |
| FMT-04 | Exit code 0 on success, non-zero on error | v1 |
| FMT-05 | Streaming line-by-line cleaned text (strip chunk wrappers as they arrive) acceptable as alternative | v2 |

## Success Criteria

- [ ] [FMT-01] `nlm generate-chat <id> <question> --format plain` executes without error
- [ ] [FMT-02] Output of `--format plain` is readable prose text matching the notebook's answer
- [ ] [FMT-03] Output contains no `wrb.fr`, `)]}'`, `\n`, `[[` JSON artifacts
- [ ] [FMT-04] Command exits 0 on success

## Constraints

- Simplest approach wins — buffer full response and print at end is acceptable
- Must not break existing streaming behavior (default)
- Primary consumer: Claude Code agents reading stdout

## Testing Strategy

- **Approach:** CLI integration test
- **Framework:** Shell script or `go test` with exec
- **Command:** `nlm generate-chat <notebook-id> "test question" --format plain`
- **Verification:** Assert output contains no JSON chunk markers

### REAL Test Definition

| Field | Value |
|-------|-------|
| **User workflow** | `nlm generate-chat <id> <question> --format plain` → capture stdout → readable prose |
| **Code paths** | Streaming response parser → text extractor → plain output writer |
| **What user sees** | Clean answer text, no JSON artifacts |
| **Protocol** | HTTPS batchexecute streaming |

### First Failing Test

- **Test name:** `test_plain_format_no_json_artifacts`
- **What it tests:** `--format plain` output contains no `wrb.fr` or `)]}'`
- **Expected failure before fix:** Command unknown flag error or raw JSON output

## Open Questions

- Where exactly in the streaming response is the text accumulated? (explore phase)
- Is there an existing text extractor we can reuse? (explore phase)
