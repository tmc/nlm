---
name: nlm
description: "Manages Google NotebookLM notebooks via the nlm CLI. Creates notebooks, uploads sources (files, URLs, stdin), generates audio/video/slides, runs chat sessions, and transforms content. Use when interacting with NotebookLM or uploading project files to a notebook."
when_to_use: "User mentions NotebookLM, wants to create a notebook, upload files to a notebook, generate audio or video overviews, create slides or presentations, chat with sources, summarize documents, or manage notebook content."
allowed-tools: Bash(*), Read, Glob, Grep, Write
argument-hint: "[action] [args...]"
---

# nlm — NotebookLM CLI

## Commands and Flags

```
!`nlm --help 2>&1`
```

Run `nlm <command>` with no args to see usage for that command. IDs are UUIDs.

## Interpreting $ARGUMENTS

| Argument | Action |
|----------|--------|
| (empty) | Run `nlm list`, ask what to do |
| `create` or `new` | Create notebook workflow |
| `upload` or `add` | Upload sources workflow |
| `chat` | Start or resume chat |
| `audio` / `video` / `slides` | Content creation workflow |
| `status` | Show notebook + sources overview |
| a notebook ID | Show details for that notebook |
| a file path or glob | Upload that file/pattern to a notebook |

## Critical Flags

- **`-y`** — Skip confirmation prompts. Always use `-y` with `rm`, `rm-source`, `rm-note` etc. in non-interactive contexts: `nlm -y rm <id>`. Without `-y`, these commands require interactive TTY input that cannot be piped.
- **`--direct-rpc`** — Required for `audio-download` and `video-download`.
- **`--authuser N`** — Select Google account (0-indexed). Also via `NLM_AUTHUSER=N`.

## Things `--help` Won't Tell You

**Sync a directory as one source** — `nlm sync` packs files into txtar natively
(no external tools), quotes files containing txtar markers so they round-trip,
chunks at 5MB, and is idempotent via a content-hash cache. Re-running only
uploads what changed:
```bash
nlm sync <notebook-id> src/                      # name derives from dir
nlm sync <notebook-id> src/ --name "project: src/"
nlm sync <notebook-id> --dry-run                 # preview plan, no upload
nlm sync <notebook-id> --force                   # re-upload even if unchanged
nlm sync <notebook-id> --json                    # NDJSON progress for scripts
```
Use `sync` over `add` whenever you'll re-upload the same tree. Use `add` for
one-shot single-file/URL uploads.

**Preview what sync will upload** — `nlm sync-pack` writes the exact txtar
bytes sync would upload, no network. Pipe through `txtar --list` or `txtar -x`
to inspect:
```bash
nlm sync-pack src/ | txtar --list
nlm sync-pack src/ > preview.txtar
nlm sync-pack src/ --chunk 2 > pt2.txtar         # pick one chunk of many
```

**Focus on specific sources** — `--source-ids` and `--source-match` scope chat,
report, and transform commands to a subset of a notebook's sources. The flags
apply to `chat`, `generate-chat`, `create-report`, `generate-report`, and every
content transform (`summarize`, `briefing`, `timeline`, etc.). They union when
both are passed:
```bash
nlm chat <nb> "what does sync do?" --source-match 'internal/sync'
nlm generate-report <nb> "Deep dive" --source-match '^nlm '
nlm summarize <nb> --source-ids a,b,c
nlm chat <nb> "..." --source-match '^132af'          # UUID-prefix match
```
`--source-match` is a Go regex matched against titles AND UUIDs; an empty match
fails fast and lists available titles.

**Rename after stdin upload** — stdin sources appear as "Pasted Text". Either use `--name` during add, or rename after:
```bash
nlm rename-source <source-id> "descriptive name"
```

**Binary upload workarounds** — if PDF/plist upload fails with 500:
```bash
pdftotext paper.pdf - | nlm add <notebook-id> -
plutil -convert xml1 -o - file.plist | nlm add <notebook-id> -
```

**Content creation takes time** — after `create-audio`, `create-video`, or `create-slides`, poll status with `nlm artifacts <id>` until ready.

**Download requires `--direct-rpc`**:
```bash
nlm --direct-rpc audio-download <id> output.wav
nlm --direct-rpc video-download <id> output.mp4
```

**Multi-account auth** — use `--authuser N` or `NLM_AUTHUSER=N` for non-default Google accounts.

**Always surface IDs** — EVERY time you display notebooks or sources, include the full UUID in your output. The user needs these IDs for follow-up commands. Never hide IDs behind a formatted table that omits them.

## Error Recovery

| Error | Fix |
|-------|-----|
| "Authentication required" | Run `nlm auth` |
| "Service unavailable" on upload | Retry after a few seconds (rate limit) |
| "Failed precondition" | Convert binary to text first (see above) |
| "upload init failed (status 500)" | Try text extraction workaround |
