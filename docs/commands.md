---
title: Command Reference
---
# Command Reference

Usage: `nlm [flags] <command> [arguments]`

Default help teaches grouped noun-first commands for notebooks, sources,
notes, artifacts, and chat administration. Legacy top-level aliases remain
available for existing scripts, but the forms below are the canonical surface.

Run `nlm <command> -h` for exact per-command usage.

## Shared flags

| Flag | Applies to | Purpose |
|------|------------|---------|
| `--auth TOKEN`, `--cookies COOKIES` | most commands | Supply credentials non-interactively |
| `--debug` | most commands | Print debug output to stderr |
| `--json` | list and sync output | Emit JSON / JSON-lines output |
| `--experimental` | hidden commands | Enable experimental commands in help and execution |
| `-y`, `--yes` | destructive commands | Skip confirmation prompts |
| `--source-ids IDS`, `--source-match REGEX` | chat, report, transforms | Limit work to selected sources |
| `--citations MODE`, `--thinking`, `--prompt-file PATH` | chat | Control streaming format and prompt input |
| `--mode MODE`, `--md`, `--import` | research | Control research mode and output |

## Notebook

| Command | Description |
|---------|-------------|
| `nlm notebook list` | List notebooks |
| `nlm notebook list --limit 25` | Show only the first 25 notebooks |
| `nlm notebook list --all` | Show every notebook on a TTY |
| `nlm notebook create "Title"` | Create a notebook |
| `nlm notebook delete NOTEBOOK_ID` | Delete a notebook |
| `nlm notebook featured` | List featured notebooks |
| `nlm notebook rename NOTEBOOK_ID "New Title"` | Rename a notebook |
| `nlm notebook emoji NOTEBOOK_ID "📚"` | Set the notebook emoji |
| `nlm notebook description NOTEBOOK_ID "Text"` | Set creator notes (text via arg or stdin; empty clears) |
| `nlm notebook cover NOTEBOOK_ID PRESET_ID` | Pick a built-in cover image by preset ID |
| `nlm notebook cover-image NOTEBOOK_ID ./image.png` | Upload a custom cover image |
| `nlm notebook unrecent NOTEBOOK_ID` | Remove from the recently-viewed list (does not delete) |

`notebook list` shows the first 10 notebooks on a TTY by default. When stdout
is piped, it emits the full list unless you pass `--limit`.

## Source

| Command | Description |
|---------|-------------|
| `nlm source list NOTEBOOK_ID` | List sources in a notebook |
| `nlm source add NOTEBOOK_ID https://example.com/article` | Add a URL source |
| `nlm source add NOTEBOOK_ID ./paper.pdf` | Add a file source |
| `nlm source add NOTEBOOK_ID "Meeting notes from March 5"` | Add a text source |
| `nlm source add NOTEBOOK_ID -` | Read newline-delimited source references from stdin |
| `nlm source sync NOTEBOOK_ID .` | Sync files into one managed source |
| `nlm source pack .` | Preview the txtar payload `source sync` would upload |
| `nlm source delete NOTEBOOK_ID SOURCE_ID` | Remove one source |
| `nlm source delete NOTEBOOK_ID -` | Remove sources from stdin |
| `nlm source rename SOURCE_ID "New Name"` | Rename a source |
| `nlm source refresh NOTEBOOK_ID SOURCE_ID` | Refresh source content |
| `nlm source check SOURCE_ID [NOTEBOOK_ID]` | Check source freshness |
| `nlm source read SOURCE_ID [NOTEBOOK_ID]` | Print the indexed text body |

`source add` accepts URLs, file paths, or literal text. When you pass `-`,
stdin is treated as one source reference per line. For multi-line text, pass a
file path or use notes instead. Use `--name` to override the title, `--replace`
to swap in a new upload, and `--mime` to override MIME detection for file
uploads.

`source sync` expands directories with tracked files by default. Add
`--include-untracked` to also include untracked, non-ignored files.

## Note

| Command | Description |
|---------|-------------|
| `nlm note list NOTEBOOK_ID` | List notes in a notebook |
| `nlm note read NOTEBOOK_ID NOTE_ID` | Show note content |
| `nlm note create NOTEBOOK_ID "Title" "Content"` | Create a note |
| `nlm note create NOTEBOOK_ID "Title" < content.md` | Create a note from stdin |
| `nlm note update NOTEBOOK_ID NOTE_ID "Content" "Title"` | Update note content and title |
| `nlm note delete NOTEBOOK_ID NOTE_ID` | Delete a note |

Note bodies are sent verbatim as Markdown; the rich-text editor in the web
UI converts to Markdown on save, so piping a `.md` file in via stdin produces
the rendering you expect without any conversion step.

## Label

Labels are NotebookLM's source-clustering primitive. The autolabel suite
generates clusters; the manual suite lets you create, rename, and attach
labels yourself.

| Command | Description |
|---------|-------------|
| `nlm label list NOTEBOOK_ID` | List labels (autolabel clusters) in a notebook |
| `nlm label generate NOTEBOOK_ID` | Recompute autolabel clusters |
| `nlm label create NOTEBOOK_ID "Name" [emoji]` | Create a manual label |
| `nlm label rename NOTEBOOK_ID LABEL_ID "New Name"` | Rename a label |
| `nlm label emoji NOTEBOOK_ID LABEL_ID "🏷️"` | Set or clear a label's emoji |
| `nlm label delete NOTEBOOK_ID LABEL_ID [LABEL_ID...]` | Delete one or more labels |
| `nlm label attach NOTEBOOK_ID LABEL_ID SOURCE_ID` | Attach a source to a label |
| `nlm label unlabeled NOTEBOOK_ID` | Apply existing labels to currently-unlabeled sources |
| `nlm label relabel-all NOTEBOOK_ID` | Re-cluster everything (the UI's "Relabel all") |

## Create

| Command | Description |
|---------|-------------|
| `nlm create-audio NOTEBOOK_ID "Conversational summary"` | Create an audio overview |
| `nlm create-video NOTEBOOK_ID "Whiteboard walkthrough"` | Create a video overview |
| `nlm create-slides NOTEBOOK_ID "Presentation summary"` | Create a slide deck |
| `nlm report-suggestions NOTEBOOK_ID` | Suggest report topics |
| `nlm create-report NOTEBOOK_ID REPORT_TYPE "Focused brief"` | Create a report artifact |

## Audio and Video

| Command | Description |
|---------|-------------|
| `nlm audio list NOTEBOOK_ID` | List audio overviews |
| `nlm audio get NOTEBOOK_ID` | Get audio overview details |
| `nlm --direct-rpc audio download NOTEBOOK_ID [FILE]` | Download the audio file |
| `nlm audio delete NOTEBOOK_ID` | Delete an audio overview |
| `nlm audio share NOTEBOOK_ID` | Share an audio overview |
| `nlm video list NOTEBOOK_ID` | List video overviews |
| `nlm video get NOTEBOOK_ID` | Get video overview details |
| `nlm --direct-rpc video download NOTEBOOK_ID [FILE]` | Download the video file |

## Artifact

| Command | Description |
|---------|-------------|
| `nlm artifact list NOTEBOOK_ID` | List artifacts in a notebook |
| `nlm artifact get ARTIFACT_ID` | Show artifact details |
| `nlm artifact update ARTIFACT_ID [NEW_TITLE]` | Rename an artifact |
| `nlm artifact delete ARTIFACT_ID` | Delete an artifact |

`artifact update` also accepts `--name` instead of a positional title.

## Guidebook

| Command | Description |
|---------|-------------|
| `nlm guidebooks` | List guidebooks |
| `nlm guidebook GUIDEBOOK_ID` | Show guidebook details |
| `nlm guidebook-details GUIDEBOOK_ID` | Show guidebook details with sections and analytics |
| `nlm guidebook-publish GUIDEBOOK_ID` | Publish a guidebook |
| `nlm guidebook-share GUIDEBOOK_ID` | Share a guidebook |
| `nlm guidebook-ask GUIDEBOOK_ID "Question"` | Ask a guidebook a question |
| `nlm guidebook-rm GUIDEBOOK_ID` | Delete a guidebook |

## Generation

| Command | Description |
|---------|-------------|
| `nlm generate-guide NOTEBOOK_ID` | Generate a notebook guide |
| `nlm source-guide NOTEBOOK_ID SOURCE_ID...` | Show source summaries and keyword chips |
| `nlm generate-chat NOTEBOOK_ID "Prompt"` | Stream a one-shot chat answer |
| `nlm audio-suggestions NOTEBOOK_ID` | Suggest audio-overview prompts |
| `nlm generate-report NOTEBOOK_ID` | Generate a multi-section report via chat |

## Chat

| Command | Description |
|---------|-------------|
| `nlm chat NOTEBOOK_ID` | Start an interactive chat session |
| `nlm chat NOTEBOOK_ID CONVERSATION_ID` | Resume an existing conversation |
| `nlm chat NOTEBOOK_ID "One-shot question"` | Ask one question without entering interactive mode |
| `nlm chat list` | List local chat sessions |
| `nlm chat list NOTEBOOK_ID` | List server-side conversations for a notebook |
| `nlm chat history NOTEBOOK_ID CONVERSATION_ID` | Show server-side conversation history |
| `nlm chat show NOTEBOOK_ID CONVERSATION_ID` | Render a local chat transcript |
| `nlm chat delete NOTEBOOK_ID` | Delete server-side chat history |
| `nlm chat config NOTEBOOK_ID SETTING [VALUE]` | Configure chat settings |
| `nlm chat instructions set NOTEBOOK_ID "Prompt"` | Set chat instructions |
| `nlm chat instructions get NOTEBOOK_ID` | Show current chat instructions |

For structured chat output, use `--citations=json`; add `--thinking` if you
also want reasoning events in the JSON-lines stream.

## Content Transformation

These commands all use:

```bash
nlm <command> NOTEBOOK_ID [SOURCE_ID...]
```

If you omit source IDs, pass `--source-ids` or `--source-match`.

| Command | Description |
|---------|-------------|
| `summarize` | Summarize content from sources |
| `rephrase` | Rephrase content from sources |
| `expand` | Expand on content from sources |
| `critique` | Critique source content |
| `brainstorm` | Brainstorm from source material |
| `verify` | Verify facts in sources |
| `explain` | Explain concepts from sources |
| `outline` | Create an outline from sources |
| `study-guide` | Generate a study guide |
| `faq` | Generate a FAQ |
| `briefing-doc` | Create a briefing document |
| `mindmap` | Generate an interactive mindmap |
| `timeline` | Create a timeline |
| `toc` | Generate a table of contents |

## Research

| Command | Description |
|---------|-------------|
| `nlm research NOTEBOOK_ID "Question"` | Run research and emit JSON-lines events |
| `nlm research NOTEBOOK_ID --mode=fast "Question"` | Use fast research mode |
| `nlm research NOTEBOOK_ID --md "Question"` | Emit the final report as markdown |
| `nlm research NOTEBOOK_ID --import "Question"` | Import discovered sources after completion |

## Sharing

| Command | Description |
|---------|-------------|
| `nlm share NOTEBOOK_ID` | Share a notebook publicly |
| `nlm share-private NOTEBOOK_ID` | Share a notebook privately |
| `nlm share-details SHARE_ID` | Show sharing details |

## Other

| Command | Description |
|---------|-------------|
| `nlm auth` | Set up authentication from a browser profile |
| `nlm auth --print-env` | Print shell export lines for the current session |
| `nlm refresh` | Refresh stored credentials |
| `nlm feedback "Message"` | Submit feedback to NotebookLM |
| `nlm mcp` | Run the MCP server on stdin/stdout |
