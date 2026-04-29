---
name: nlm
description: "Manages Google NotebookLM notebooks via the nlm CLI. Use for creating notebooks, listing and syncing sources, uploading files/URLs/text, chatting with sources, generating reports/audio/video/slides, running research, managing labels, editing notebook metadata (title/emoji/description/cover), and managing notebook content."
when_to_use: "User mentions NotebookLM, nlm, notebook IDs, source upload/sync, chat with sources, report/audio/video/slides generation, research, labels/autolabel clusters, notebook metadata (rename/emoji/description/cover), or managing NotebookLM notes/artifacts."
allowed-tools: Bash(*), Read, Glob, Grep, Write
argument-hint: "[action] [args...]"
---

# nlm — NotebookLM CLI

## Command Discovery

Run `nlm --help` for the canonical command tree. Run `nlm <command> --help`
for command-local flags. The stable surface is noun-first: `notebook`,
`source`, `note`, `artifact`, and `chat` groups.

For a compact command map, read `reference/commands.md` only when needed.
Always prefer live help output when it disagrees with the reference.

## Interpreting $ARGUMENTS

| Argument | Action |
|----------|--------|
| (empty) | Run `nlm notebook list`, then ask what to do |
| `create` or `new` | Create a notebook with `nlm notebook create` |
| `upload` or `add` | Add one-off sources with `nlm source add` |
| `sync` | Sync a directory as a managed source with `nlm source sync` |
| `chat` | Start, resume, or run one-shot chat |
| `research` | Run `nlm research` and choose fast/deep mode if needed |
| `audio` / `video` / `slides` / `report` | Use the corresponding create or generation command |
| `status` | Show notebook, sources, artifacts, and recent chats |
| a notebook ID | Show details for that notebook |
| a file path or glob | Upload that file/pattern to a notebook |

## Critical Practices

- Surface full UUIDs for notebooks, sources, conversations, notes, and artifacts in responses. Follow-up commands need them.
- Use `-y` for destructive operations in non-interactive contexts, for example `nlm -y notebook delete <id>`.
- Use `nlm auth --authuser N` or `NLM_AUTHUSER=N` for non-default Google accounts.
- Use `--direct-rpc` for `audio-download` and `video-download`.
- Treat compatibility aliases (`nlm list`, `nlm sources`, `nlm rm-source`) as runnable but prefer canonical forms in new guidance.

## Common Workflows

**List notebooks**
```bash
nlm notebook list
nlm notebook list --limit 25
nlm notebook list --all
```

**Add one-off sources** — use `source add` for files, URLs, and direct text.
Pass `-` to read newline-delimited source references from stdin.
```bash
nlm source add <notebook-id> https://example.com/article
nlm source add <notebook-id> ./paper.pdf
nlm source add --name "API notes" <notebook-id> ./notes.txt
printf '%s\n' ./a.pdf https://example.com/b | nlm source add <notebook-id> -
```

**Sync a directory as one source** — `source sync` packs files into txtar,
quotes nested txtar markers, chunks large payloads, and skips unchanged chunks
using a content-hash cache. Use it whenever the same tree will be re-uploaded.
Use `source add` for one-shot single-file/URL uploads.
```bash
nlm source sync <notebook-id> src/
nlm source sync --name "project: src/" <notebook-id> src/
nlm source sync --dry-run <notebook-id> .
nlm source sync --force <notebook-id> ./docs ./notes
nlm source sync --json <notebook-id> .
```

**Preview what sync will upload** — `source pack` writes the exact txtar
bytes sync would upload, no network. Pipe through `txtar --list` or `txtar -x`
to inspect:
```bash
nlm source pack src/ | txtar --list
nlm source pack src/ > preview.txtar
nlm source pack --chunk 2 src/ > pt2.txtar
```

**Focus on specific sources** — `--source-ids` and `--source-match` scope
chat, `generate-chat`, `generate-report`, `source-guide`, and content
transforms. `--source-match` is a Go regex matched against titles and UUIDs.
```bash
nlm chat --source-match 'internal/sync' <notebook-id> "What changed?"
nlm generate-chat --source-ids a,b,c <notebook-id> "Summarize these"
nlm summarize --source-match '^spec/' <notebook-id>
nlm source list <notebook-id> | grep Q3 | nlm chat --source-ids - <notebook-id> "Risks?"
```

**Chat and continuation**
```bash
nlm chat <notebook-id>
nlm chat <notebook-id> "What are the main conclusions?"
nlm generate-chat --conversation <conversation-id> <notebook-id> "Follow up"
nlm chat show --citations tail <notebook-id> <conversation-id>
```

**Research**
```bash
nlm research <notebook-id> "What changed in the source set?"
nlm research --mode fast <notebook-id> "Which docs should I read first?"
nlm research --md <notebook-id> "Write a concise brief" > report.md
nlm research --import <notebook-id> "Find source material"
```

**Content creation** — creation may take time. Poll with `artifact list`,
`audio-list`, or `video-list`.
```bash
nlm create-audio <notebook-id> "Conversational, focus on key decisions"
nlm create-video <notebook-id> "Whiteboard walkthrough"
nlm create-slides <notebook-id> "Presentation summary"
nlm generate-report --sections 3 <notebook-id>
nlm artifact list <notebook-id>
nlm --direct-rpc audio-download <notebook-id> output.wav
nlm --direct-rpc video-download <notebook-id> output.mp4
```

**Rename after stdin upload** — stdin text defaults to "Pasted Text"; use
`--name` during upload or rename after:
```bash
nlm source rename <source-id> "descriptive name"
```

**Notebook metadata** — title, emoji, description, and cover are separate
commands. `cover` takes a built-in preset ID; `cover-image` uploads a custom
image. `unrecent` only hides from the recents list, it does not delete.
```bash
nlm notebook rename <notebook-id> "New Title"
nlm notebook emoji <notebook-id> "📓"
nlm notebook description <notebook-id> "One-line summary"
echo "long description" | nlm notebook description <notebook-id>
nlm notebook cover <notebook-id> 4
nlm notebook cover-image <notebook-id> ./cover.png
nlm notebook unrecent <notebook-id>
```

**Labels (autolabel clusters)** — labels are server-side clusters over
sources. `generate` and `relabel-all` are heavy server jobs (relabel-all can
exceed the 60s deadline on large notebooks); `unlabeled` only touches
sources without a label. `attach` takes one source per call.
```bash
nlm label list <notebook-id>
nlm label generate <notebook-id>
nlm label create <notebook-id> "Important" "⭐"
nlm label rename <notebook-id> <label-id> "New Name"
nlm label emoji <notebook-id> <label-id> "🐛"
nlm label delete <notebook-id> <label-id> [<label-id>...]
nlm label unlabeled <notebook-id>
nlm label relabel-all <notebook-id>
nlm label attach <notebook-id> <label-id|name> <source-id|name>
```

**Discover sources vs chat** — `discover-sources` calls a server-driven
source-discovery RPC (Es3dTe) that returns ranked source IDs for a query.
If the server rejects it (error or transient code-13), the CLI falls back
to a regular chat call asking the model to list relevant sources. Use it
to pick `--source-ids` for a follow-up; use `nlm chat` when you want a
narrative answer rather than just IDs.
```bash
nlm discover-sources <notebook-id> "Q3 revenue assumptions"
```

## Source Freshness Strategy

Pick the lightest tool that does the job:

- `nlm source check <source-id> [notebook-id]` — Drive-only. Asks Google
  whether the indexed copy is still current. No re-index, no upload.
  Use to decide whether anything else is needed.
- `nlm source refresh <notebook-id> <source-id>` — Drive-only. Re-indexes
  the existing source in place. Use when `check` reports stale and the
  source is still a Google Drive document.
- Re-upload — for non-Drive sources (files, URLs, pasted text) `check`
  and `refresh` do not apply. Use `nlm source delete` then `nlm source add`,
  or for a synced tree run `nlm source sync` (it auto-detects changed
  chunks; `--force` to re-upload unchanged content).

**Binary upload workarounds** — if a binary upload fails, convert to text:
```bash
pdftotext paper.pdf - | nlm source add --name "paper text" <notebook-id> -
plutil -convert xml1 -o - file.plist | nlm source add --name "plist text" <notebook-id> -
```

## Error Recovery

| Error | Fix |
|-------|-----|
| "Authentication required" | Run `nlm auth` |
| "Service unavailable" on upload | Retry after a few seconds (rate limit) |
| "source limit reached" or "Failed precondition" on add | Remove unused sources or use a smaller target notebook |
| "upload init failed (status 500)" | Try text extraction workaround |
| `--source-match matched no sources` | Re-run `nlm source list <notebook-id>` and adjust the regex |
