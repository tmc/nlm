---
title: Command Reference
---
# Command Reference

Usage: `nlm [flags] <command> [arguments]`

## Global Flags

| Flag | Env Var | Description |
|------|---------|-------------|
| `--debug` | `NLM_DEBUG` | Enable debug output to stderr |
| `--auth TOKEN` | `NLM_AUTH_TOKEN` | Authentication token |
| `--cookies COOKIES` | `NLM_COOKIES` | Session cookies |
| `--profile NAME` | `NLM_BROWSER_PROFILE` | Chrome profile name |
| `--chunked` | | Use chunked response format |
| `--direct-rpc` | | Use direct RPC calls (required for audio/video download) |
| `-y`, `--yes` | | Skip confirmation prompts |
| `--mime TYPE` | | MIME type for source content |
| `--name NAME`, `-n` | | Custom name for added sources |
| `--thinking` | | Show thinking headers in chat responses |
| `--verbose`, `-v` | | Show full thinking traces in chat responses |
| `--history` | | Show previous chat conversation on start |
| `--debug-dump-payload` | | Dump raw JSON payload and exit |
| `--debug-parsing` | | Show protobuf parsing details |
| `--debug-field-mapping` | | Show JSON-to-protobuf field mapping |
| `--skip-sources` | `NLM_SKIP_SOURCES` | Skip source fetching for chat |

## Notebooks

### list, ls

List all notebooks.

```bash
nlm list
nlm ls
```

### create

Create a new notebook.

```bash
nlm create "My Research"
```

### rm

Delete a notebook.

```bash
nlm rm NOTEBOOK_ID
nlm -y rm NOTEBOOK_ID  # skip confirmation
```

### analytics

Show notebook analytics.

```bash
nlm analytics NOTEBOOK_ID
```

### list-featured

List featured/recommended notebooks.

```bash
nlm list-featured
```

## Sources

### sources

List sources in a notebook.

```bash
nlm sources NOTEBOOK_ID
```

### add

Add a source to a notebook. Accepts URLs, file paths, or `-` for stdin.

```bash
nlm add NOTEBOOK_ID "https://example.com/article"
nlm add NOTEBOOK_ID ./paper.pdf
nlm add NOTEBOOK_ID - --mime text/plain < notes.txt
nlm add NOTEBOOK_ID -n "Custom Name" ./data.csv
```

Supported inputs:
- URLs (web pages, YouTube, Google Docs)
- Local files (PDF, DOCX, PPTX, XLSX, TXT, etc.)
- Stdin with `-` (use `--mime` to set content type)
- Plain text strings

### rm-source

Remove a source from a notebook.

```bash
nlm rm-source NOTEBOOK_ID SOURCE_ID
```

### rename-source

Rename a source.

```bash
nlm rename-source SOURCE_ID "New Name"
```

### refresh-source

Refresh a URL source's content.

```bash
nlm refresh-source NOTEBOOK_ID SOURCE_ID
```

### check-source

Check whether a source's content is up to date.

```bash
nlm check-source SOURCE_ID
```

### discover-sources

Discover relevant sources based on a query.

```bash
nlm discover-sources NOTEBOOK_ID "machine learning"
```

## Notes

### notes

List notes in a notebook.

```bash
nlm notes NOTEBOOK_ID
```

### read-note

Display the full content of a specific note.

```bash
nlm read-note NOTEBOOK_ID NOTE_ID
```

### new-note

Create a new note. Content can be passed as an argument or piped via stdin.

```bash
nlm new-note NOTEBOOK_ID "Title" "Content here"
nlm new-note NOTEBOOK_ID "Title" < content.md
```

### update-note

Update a note's content and title.

```bash
nlm update-note NOTEBOOK_ID NOTE_ID "New content" "New Title"
```

### rm-note

Delete a note.

```bash
nlm rm-note NOTEBOOK_ID NOTE_ID
```

## Content Creation

### create-audio

Create an audio overview with generation instructions.

```bash
nlm create-audio NOTEBOOK_ID "Conversational tone, focus on key findings"
```

### create-video

Create a video overview.

```bash
nlm create-video NOTEBOOK_ID "Educational style with key charts"
```

### create-slides

Create a slide deck presentation.

```bash
nlm create-slides NOTEBOOK_ID "Make a detailed presentation on the key findings"
```

### create-infographic

Create an infographic from notebook sources.

```bash
nlm create-infographic NOTEBOOK_ID "Create a visual summary of the key findings"
```

## Audio

### audio-list

List audio overviews for a notebook.

```bash
nlm audio-list NOTEBOOK_ID
```

### audio-get

Get audio overview details and status.

```bash
nlm audio-get NOTEBOOK_ID
```

### audio-download

Download the audio file. Requires `--direct-rpc`.

```bash
nlm --direct-rpc audio-download NOTEBOOK_ID
nlm --direct-rpc audio-download NOTEBOOK_ID output.mp3
```

### audio-rm

Delete an audio overview.

```bash
nlm audio-rm NOTEBOOK_ID
```

### audio-share

Share an audio overview publicly.

```bash
nlm audio-share NOTEBOOK_ID
```

### audio-interactive

Start a live interactive audio session via WebRTC. Requires `NLM_EXPERIMENTAL=1`.

```bash
NLM_EXPERIMENTAL=1 nlm audio-interactive NOTEBOOK_ID
```

## Video

### video-list

List video overviews for a notebook.

```bash
nlm video-list NOTEBOOK_ID
```

### video-download

Download a video file. Requires `--direct-rpc`.

```bash
nlm --direct-rpc video-download NOTEBOOK_ID
nlm --direct-rpc video-download NOTEBOOK_ID output.mp4
```

## Artifacts

### artifacts, list-artifacts

List artifacts in a notebook.

```bash
nlm artifacts NOTEBOOK_ID
```

### get-artifact

Get artifact details.

```bash
nlm get-artifact ARTIFACT_ID
```

### rename-artifact

Rename an artifact.

```bash
nlm rename-artifact ARTIFACT_ID "New Title"
```

### delete-artifact

Delete an artifact.

```bash
nlm delete-artifact ARTIFACT_ID
```

## Guidebooks

### guidebooks

List all guidebooks.

```bash
nlm guidebooks
```

### guidebook

Get guidebook content and details.

```bash
nlm guidebook GUIDEBOOK_ID
```

### guidebook-publish

Publish a guidebook.

```bash
nlm guidebook-publish GUIDEBOOK_ID
```

### guidebook-share

Share a guidebook.

```bash
nlm guidebook-share GUIDEBOOK_ID
```

### guidebook-ask

Ask a guidebook a question.

```bash
nlm guidebook-ask GUIDEBOOK_ID "What are the key findings?"
```

### guidebook-rm

Delete a guidebook.

```bash
nlm guidebook-rm GUIDEBOOK_ID
```

## Chat & Generation

### chat

Interactive chat with a notebook's sources. Supports persistent sessions with history.

```bash
nlm chat NOTEBOOK_ID                    # new session
nlm chat NOTEBOOK_ID CONVERSATION_ID    # resume session
nlm chat NOTEBOOK_ID "one-shot question" # single question, no session
nlm chat --history NOTEBOOK_ID          # show previous messages
nlm chat --thinking NOTEBOOK_ID         # show reasoning headers
nlm chat --verbose NOTEBOOK_ID          # show full reasoning traces
```

### generate-chat

One-shot chat generation (non-interactive).

```bash
nlm generate-chat NOTEBOOK_ID "What are the main themes?"
```

### chat-list

List chat sessions. Without a notebook ID, lists local sessions; with one, lists server-side conversations.

```bash
nlm chat-list
nlm chat-list NOTEBOOK_ID
```

### delete-chat

Delete server-side chat history.

```bash
nlm delete-chat NOTEBOOK_ID
```

### chat-config

Configure chat settings for a notebook.

```bash
nlm chat-config NOTEBOOK_ID goal "Research analysis"
nlm chat-config NOTEBOOK_ID length "detailed"
```

### set-instructions

Set system instructions for a notebook's chat.

```bash
nlm set-instructions NOTEBOOK_ID "Always cite sources and be concise"
```

### get-instructions

Show current system instructions.

```bash
nlm get-instructions NOTEBOOK_ID
```

### generate-guide

Generate a comprehensive notebook guide.

```bash
nlm generate-guide NOTEBOOK_ID
```

### generate-outline

Generate a content outline.

```bash
nlm generate-outline NOTEBOOK_ID
```

### generate-section

Generate a new section.

```bash
nlm generate-section NOTEBOOK_ID
```

### generate-magic

Generate a "magic view" from specific sources.

```bash
nlm generate-magic NOTEBOOK_ID SOURCE_ID1 SOURCE_ID2
```

## Content Transformation

These commands operate on specific sources within a notebook. All accept one or more source IDs.

```bash
nlm <command> NOTEBOOK_ID SOURCE_ID [SOURCE_ID...]
```

| Command | Description |
|---------|-------------|
| `summarize` | Create a concise summary |
| `rephrase` | Rephrase for clarity |
| `expand` | Expand with more detail |
| `critique` | Critical analysis |
| `brainstorm` | Generate ideas |
| `verify` | Fact-check content |
| `explain` | Explain concepts |
| `outline` | Create a structured outline |
| `study-guide` | Generate a study guide |
| `faq` | Generate FAQ |
| `briefing-doc` | Create an executive briefing |
| `mindmap` | Generate an interactive mind map |
| `timeline` | Create a chronological timeline |
| `toc` | Generate a table of contents |

## Research

### research

Start deep research on a topic and poll for results.

```bash
nlm research NOTEBOOK_ID "What are the implications of these findings?"
```

## Sharing

### share

Share a notebook publicly.

```bash
nlm share NOTEBOOK_ID
```

### share-private

Share a notebook privately.

```bash
nlm share-private NOTEBOOK_ID
```

### share-details

Get details about a shared notebook.

```bash
nlm share-details SHARE_ID
```

## System

### auth

Set up authentication by extracting browser cookies.

```bash
nlm auth
nlm auth --profile "Work"
nlm auth --all
nlm auth --cdp-url ws://localhost:9222
nlm auth --authuser 1  # use secondary Google account
```

### refresh

Refresh authentication credentials.

```bash
nlm refresh
```

### hb

Send a heartbeat to verify connectivity.

```bash
nlm hb
```

### feedback

Submit feedback to NotebookLM.

```bash
nlm feedback "Great tool!"
```

### mcp

Start the MCP server on stdin/stdout. See [MCP Server](mcp.md).

```bash
nlm mcp
```
