---
name: nlm
description: "Manage Google NotebookLM notebooks — create notebooks, upload sources, chat, generate audio/video/slides, manage artifacts. Use when the user wants to interact with NotebookLM or upload project files to a notebook."
allowed-tools: Bash(*), Read, Glob, Grep, Write
argument-hint: "[action] [args...]"
---

# NLM — NotebookLM CLI Skill

You are helping the user manage Google NotebookLM notebooks via the `nlm` CLI tool.

## Quick Reference

```
nlm list                           # List notebooks
nlm create <title>                 # Create notebook
nlm rm -y <id>                     # Delete notebook
nlm analytics <id>                 # Show notebook analytics
```

### Sources
```
nlm sources <id>                   # List sources in notebook
nlm add <id> <file|url|->         # Add source (use --name to set title)
nlm rm-source -y <id> <src-id>    # Remove source
nlm rename-source <src-id> <name>  # Rename source
nlm refresh-source <id> <src-id>   # Refresh source content
nlm check-source <src-id>          # Check source freshness
nlm discover-sources <id> <query>  # Discover relevant sources
```

### Notes
```
nlm notes <id>                     # List notes
nlm read-note <id> <note-id>      # Read full note content
nlm new-note <id> <title>          # Create new note
nlm update-note <id> <note-id> <content> <title>  # Edit note
nlm rm-note <id> <note-id>         # Remove note
```

### Content Creation
```
nlm create-audio <id> <instr>      # Create audio overview
nlm create-video <id> <instr>      # Create video overview
nlm create-slides <id> <instr>     # Create slide deck
```

### Audio
```
nlm audio-list <id>                # List audio overviews with status
nlm audio-get <id>                 # Get audio overview details
nlm audio-download <id> [file]     # Download audio (needs --direct-rpc)
nlm audio-rm <id>                  # Delete audio overview
nlm audio-share <id>               # Share audio overview
```

### Video
```
nlm video-list <id>                # List video overviews with status
nlm video-download <id> [file]     # Download video (needs --direct-rpc)
```

### Artifacts
```
nlm artifacts <id>                 # List artifacts in notebook
nlm get-artifact <artifact-id>     # Get artifact details
nlm rename-artifact <id> <title>   # Rename artifact
nlm delete-artifact <id>           # Delete artifact
```

### Guidebooks
```
nlm guidebooks                     # List all guidebooks
nlm guidebook <id>                 # Get guidebook content
nlm guidebook-publish <id>         # Publish a guidebook
nlm guidebook-share <id>           # Share a guidebook
nlm guidebook-ask <id> <question>  # Ask a guidebook
nlm guidebook-rm <id>              # Delete a guidebook
```

### Chat & Generation
```
nlm chat <id>                      # Interactive chat session
nlm generate-chat <id> <prompt>    # One-shot chat generation
nlm chat-list [id]                 # List chat sessions
nlm delete-chat <id>               # Delete server-side chat history
nlm chat-config <id> <setting> [value]  # Configure chat settings
nlm set-instructions <id> <text>   # Set notebook chat instructions
nlm get-instructions <id>          # Show current instructions
nlm generate-guide <id>            # Comprehensive guide
nlm generate-outline <id>          # Content outline
nlm generate-section <id>          # New section
nlm generate-magic <id> <src-ids>  # Magic view from sources
```

### Content Transformation
```
nlm summarize <id> <source-ids...>    # Summarize sources
nlm explain <id> <source-ids...>      # Explain concepts
nlm rephrase <id> <source-ids...>     # Rephrase content
nlm expand <id> <source-ids...>       # Expand on content
nlm critique <id> <source-ids...>     # Critique content
nlm brainstorm <id> <source-ids...>   # Brainstorm ideas
nlm verify <id> <source-ids...>       # Verify facts
nlm outline <id> <source-ids...>      # Create outline
nlm study-guide <id> <source-ids...>  # Study guide
nlm faq <id> <source-ids...>          # Generate FAQ
nlm briefing-doc <id> <source-ids...> # Briefing document
nlm mindmap <id> <source-ids...>      # Interactive mindmap
nlm timeline <id> <source-ids...>     # Timeline from sources
nlm toc <id> <source-ids...>          # Table of contents
```

### Sharing
```
nlm share <id>                     # Share notebook publicly
nlm share-private <id>             # Share privately
nlm share-details <share-id>       # Get share details
```

### Research
```
nlm research <id> <query>          # Deep research and poll for results
```

### System
```
nlm auth                           # Setup authentication (opens browser)
nlm auth --authuser 1              # Auth with secondary Google account
nlm refresh                        # Refresh auth credentials
nlm mcp                            # Start MCP server (stdin/stdout)
nlm hb                             # Heartbeat check
nlm feedback <msg>                 # Submit feedback
```

## Interpreting $ARGUMENTS

Parse `$ARGUMENTS` as the action:

| Argument | Action |
|----------|--------|
| (empty) | Show notebooks with `nlm list`, ask what to do |
| `create` or `new` | Create a notebook workflow |
| `upload` or `add` | Upload sources workflow |
| `sources` or `list sources` | List sources in a notebook |
| `chat` | Start or resume chat |
| `audio` | Audio overview workflow |
| `video` | Video overview workflow |
| `slides` | Slide deck creation workflow |
| `status` | Show notebook + sources overview |
| a notebook ID | Show details for that notebook |
| a file path or glob | Upload that file/pattern to a notebook |

## Workflows

### 1. Create Notebook from Project

1. Determine a good title from the project (repo name, package name, or ask)
2. `nlm create "<title>"` — capture the notebook ID from stdout
3. Gather source files to upload (see Upload Sources below)
4. Report the notebook ID so the user can reference it later

### 2. Upload Sources

**Single file:**
```bash
nlm add <notebook-id> path/to/file.go
nlm add <notebook-id> --name "Custom Title" path/to/file.go
```

**Multiple files — use txtar bundling:**
```bash
txtar-c -a src/ 2>/dev/null | nlm add <notebook-id> -
txtar-c -a . 2>/dev/null | nlm add <notebook-id> -
```

**URL sources:**
```bash
nlm add <notebook-id> "https://example.com/article"
```

**Text from stdin:**
```bash
echo "Some text content" | nlm add <notebook-id> -
```

### 3. Binary Files (PDF, images)

```bash
nlm add <notebook-id> paper.pdf
```

**Workaround if binary upload fails:**
```bash
pdftotext paper.pdf - | nlm add <notebook-id> -
plutil -convert xml1 -o - file.plist | nlm add <notebook-id> -
```

### 4. Bulk Upload Project Files

Focus on source code, docs, and config. Skip binaries, vendor, node_modules, .git, lock files.

```bash
txtar-c -a . 2>/dev/null | nlm add <notebook-id> -
```

Name the source after upload:
```bash
nlm rename-source <source-id> "project-name source code"
```

### 5. Chat with Notebook

```bash
nlm chat <notebook-id>                          # Interactive session
nlm generate-chat <notebook-id> "question"      # One-shot
nlm set-instructions <notebook-id> "Be concise" # Persistent instructions
```

### 6. Content Creation

```bash
nlm create-audio <notebook-id> "Focus on the architecture"
nlm create-video <notebook-id> "Explain the key concepts"
nlm create-slides <notebook-id> "Detailed presentation on findings"

# Check status (generation takes time)
nlm audio-list <notebook-id>
nlm video-list <notebook-id>
nlm artifacts <notebook-id>

# Download when ready
nlm --direct-rpc audio-download <notebook-id> overview.wav
nlm --direct-rpc video-download <notebook-id> overview.mp4
```

### 7. Content Transformation

```bash
nlm summarize <notebook-id> <source-id-1> <source-id-2>
nlm faq <notebook-id> <source-id>
nlm mindmap <notebook-id> <source-id>
nlm timeline <notebook-id> <source-id-1> <source-id-2>
```

## Important Notes

- **Authentication**: If you see "Authentication required", run `nlm auth` first
- **Notebook IDs** and **Source IDs** are UUIDs
- **`-y`** skips confirmation prompts for destructive operations
- **`--debug`** shows detailed request/response info
- **`--direct-rpc`** required for audio/video download commands
- **`--authuser N`** for multi-account Google profiles
- **txtar-c** must be on PATH: `go install golang.org/x/exp/cmd/txtar-c@latest`
- Always show notebook/source IDs from command output so the user can reference them

## Error Handling

| Error | Likely Cause | Fix |
|-------|-------------|-----|
| "Authentication required" | No auth or expired | Run `nlm auth` |
| "Service unavailable" on upload | Rate limiting | Retry after a few seconds |
| "Failed precondition" on plist/XML | File type not accepted | Convert to text first |
| "upload init failed (status 500)" | Binary file rejected | Try text extraction workaround |
