---
name: nlm
description: "Manages Google NotebookLM notebooks via the nlm CLI. Creates notebooks, uploads sources (files, URLs, stdin), generates audio/video/slides, runs chat sessions, and transforms content. Use when interacting with NotebookLM or uploading project files to a notebook."
when_to_use: "User mentions NotebookLM, wants to create a notebook, upload files to a notebook, generate audio or video overviews, create slides or presentations, chat with sources, summarize documents, or manage notebook content."
allowed-tools: Bash(*), Read, Glob, Grep, Write
argument-hint: "[action] [args...]"
---

# nlm — NotebookLM CLI

Manages Google NotebookLM notebooks. For the full command list, see [reference/commands.md](reference/commands.md).

## Core Commands

```
nlm list                                    # List notebooks
nlm create <title>                          # Create notebook
nlm sources <id>                            # List sources
nlm add <id> <file|url|->                  # Add source
nlm chat <id>                               # Interactive chat
nlm generate-chat <id> <prompt>             # One-shot question
nlm create-audio <id> <instructions>        # Audio overview
nlm create-video <id> <instructions>        # Video overview
nlm create-slides <id> <instructions>       # Slide deck
```

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

## Workflows

### Create Notebook and Upload Sources

```bash
# Create
nlm create "My Project"
# => notebook-id

# Upload single files
nlm add <notebook-id> paper.pdf
nlm add <notebook-id> "https://example.com/article"

# Upload project tree via txtar
txtar-c -a . 2>/dev/null | nlm add <notebook-id> -

# Name the source
nlm rename-source <source-id> "project source code"
```

For binary upload failures, convert to text first:
```bash
pdftotext paper.pdf - | nlm add <notebook-id> -
```

### Chat

```bash
nlm chat <notebook-id>                          # Interactive session
nlm generate-chat <notebook-id> "question"      # One-shot
nlm set-instructions <notebook-id> "Be concise" # Persistent instructions
```

### Content Creation

```bash
nlm create-audio <notebook-id> "Focus on the architecture"
nlm create-video <notebook-id> "Explain the key concepts"
nlm create-slides <notebook-id> "Detailed presentation on findings"

# Check status (generation takes time)
nlm artifacts <notebook-id>

# Download when ready
nlm --direct-rpc audio-download <notebook-id> overview.wav
nlm --direct-rpc video-download <notebook-id> overview.mp4
```

### Content Transformation

All take `<notebook-id> <source-ids...>`:
```bash
nlm summarize <notebook-id> <src-id>
nlm faq <notebook-id> <src-id>
nlm mindmap <notebook-id> <src-id>
nlm timeline <notebook-id> <src-id-1> <src-id-2>
nlm study-guide <notebook-id> <src-id>
nlm briefing-doc <notebook-id> <src-id>
```

## Key Facts

- **IDs** are UUIDs: `a1b2c3d4-e5f6-7890-abcd-ef1234567890`
- **`-y`** skips confirmation prompts for destructive operations
- **`--direct-rpc`** required for audio/video download
- **`--authuser N`** for multi-account Google profiles
- **txtar-c** bundles files efficiently: `go install golang.org/x/exp/cmd/txtar-c@latest`
- Always surface notebook/source IDs so the user can reference them
- If auth fails, run `nlm auth` (opens browser for cookie extraction)

## Error Handling

| Error | Fix |
|-------|-----|
| "Authentication required" | Run `nlm auth` |
| "Service unavailable" on upload | Retry after a few seconds (rate limit) |
| "Failed precondition" on plist/XML | Convert to text: `plutil -convert xml1 -o -` |
| "upload init failed (status 500)" | Try text extraction workaround |
