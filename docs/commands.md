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
| `--authuser N` | most commands | Select a Google account in a multi-account browser profile |
| `--cdp-url URL` | most commands | Use an authenticated browser debugging session when needed |
| `--debug` | most commands | Print debug output to stderr |
| `--json` | list and sync output | Emit JSON / JSON-lines output |
| `--experimental` | hidden commands | Enable experimental commands in help and execution |
| `-y`, `--yes` | destructive commands | Skip confirmation prompts |
| `--source-ids IDS`, `--source-match REGEX` | chat, report, transforms | Limit work to selected sources |
| `--citations MODE`, `--thinking`, `--prompt-file PATH` | chat, generate-chat | Control streaming format and prompt input |
| `--mode MODE`, `--md`, `--import` | research | Control research mode and output |

## Command signatures

The following signatures are generated from the executable command
specification.

<!-- BEGIN GENERATED COMMAND SIGNATURES -->
### Notebook

| Command | Description |
| --- | --- |
| `nlm notebook list [flags]` | List all notebooks |
| `nlm notebook create <title>` | Create a new notebook |
| `nlm notebook delete [flags] <notebook-id>` | Delete a notebook |
| `nlm notebook rename <notebook-id> <new-title>` | Rename a notebook |
| `nlm notebook emoji <notebook-id> <emoji>` | Change notebook emoji |
| `nlm notebook description <notebook-id> [text]` | Set notebook description / creator notes (text via arg or stdin; empty clears) |
| `nlm notebook cover <notebook-id> <preset-id>` | Pick a built-in cover image (preset ID; HAR-captured value: 4. Other IDs uncatalogued) |
| `nlm notebook cover-image <notebook-id> <image-path>` | Upload a custom cover image and associate it with the notebook |
| `nlm notebook unrecent <notebook-id>` | Remove a notebook from the recently-viewed list (does not delete it) |
| `nlm notebook featured [flags]` | List featured notebooks |
| `nlm analytics [flags] <notebook-id>` | Show notebook analytics time series |

### Source

| Command | Description |
| --- | --- |
| `nlm source list [flags] <notebook-id>` | List sources in notebook |
| `nlm source add [flags] <notebook-id> <source...>` | Add one or more sources (files, URLs, or text; pass '-' to stream stdin as a single source) |
| `nlm source sync [flags] <notebook-id> [path...]` | Bundle local files into a txtar source and keep it in sync (auto-chunks at 5MB; see --help) |
| `nlm source pack [flags] [path...]` | Preview the txtar bytes that sync would upload (offline) |
| `nlm source delete [flags] <notebook-id> <source-id\|-\|a,b,c>` | Remove one or more sources (pass '-' to read newline-delimited IDs from stdin) |
| `nlm source rename <source-id> <new-name>` | Rename a source |
| `nlm source refresh <notebook-id> <source-id>` | Refresh source content |
| `nlm source check <notebook-id> <source-id>` | Check source freshness (Google-Drive-only; notebook-id enables client-side source-type validation) |
| `nlm source read [--format text\|markdown\|html\|json\|raw\|prototext] <notebook-id> <source-id>` | Read a source body |
| `nlm discover-sources [flags] <notebook-id> <query>` | Discover relevant sources via Es3dTe (chat fallback if the server rejects) |

### Note

| Command | Description |
| --- | --- |
| `nlm note list [flags] <notebook-id>` | List notes in notebook |
| `nlm note read [--format text\|markdown\|html] [--out file] [--open] <notebook-id> <note-id>` | Read full note content |
| `nlm note create <notebook-id> <title> [--content TEXT \| --content-file FILE]` | Create new note (content via arg or stdin) |
| `nlm note update <notebook-id> <note-id> [--title TITLE] [--content TEXT \| --content-file FILE]` | Edit note content and title |
| `nlm note delete [flags] <notebook-id> <note-id>` | Remove a note from a notebook |

### Label

| Command | Description |
| --- | --- |
| `nlm label list [flags] <notebook-id>` | List labels (autolabel clusters) in a notebook |
| `nlm label generate [flags] <notebook-id>` | Recompute autolabel clusters for a notebook |
| `nlm label create [flags] <notebook-id> <name> [emoji]` | Create a new manual label on a notebook |
| `nlm label rename <notebook-id> <label-id> <new-name>` | Rename an existing label |
| `nlm label emoji <notebook-id> <label-id> <emoji>` | Set or clear the emoji on a label |
| `nlm label delete <notebook-id> <label-id> [<label-id>...]` | Delete one or more labels by ID |
| `nlm label unlabeled [flags] <notebook-id>` | Apply existing labels to currently-unlabeled sources |
| `nlm label relabel-all [flags] <notebook-id>` | Re-cluster everything (UI's "Relabel all") |
| `nlm label attach <notebook-id> <label-id\|name> <source-id\|name>` | Attach a source to a label (single source per call) |

### Create

| Command | Description |
| --- | --- |
| `nlm app create [flags] <notebook-id> <instructions...>` | Create a generated app artifact |
| `nlm mindmap create [flags] <notebook-id> <instructions...>` | Create a generated mind map artifact |
| `nlm create-audio [flags] <notebook-id> <instructions...>` | Create audio overview |
| `nlm create-video [flags] <notebook-id> <instructions...>` | Create video overview |
| `nlm app-create [flags] <notebook-id> <instructions...>` | Create a generated app artifact |
| `nlm mindmap-create [flags] <notebook-id> <instructions...>` | Create a generated mind map artifact |
| `nlm create-slides [flags] <notebook-id> [instructions...]` | Create slide deck |
| `nlm create-report [flags] <notebook-id> <report-type> [description...]` | Create a report artifact (run report-suggestions for valid types) |

### Audio

| Command | Description |
| --- | --- |
| `nlm audio list [flags] <notebook-id>` | List audio overviews for a notebook |
| `nlm audio create [flags] <notebook-id> <instructions...>` | Create audio overview |
| `nlm audio get <notebook-id>` | Get audio overview details |
| `nlm audio download <notebook-id> [filename]` | Download audio file |
| `nlm audio delete [flags] <notebook-id>` | Delete audio overview |
| `nlm audio share <notebook-id>` | Share audio overview |

### Video

| Command | Description |
| --- | --- |
| `nlm video create [flags] <notebook-id> <instructions...>` | Create video overview |

### Deck

| Command | Description |
| --- | --- |
| `nlm deck create [flags] <notebook-id> [instructions...]` | Create slide deck |
| `nlm deck download [flags] <notebook-id>` | Download a slide deck (PDF/PPTX) |

### Artifact

| Command | Description |
| --- | --- |
| `nlm artifact list [flags] <notebook-id>` | List artifacts in notebook |
| `nlm artifact get <artifact-id>` | Get artifact details |
| `nlm artifact read <artifact-id>` | Print a text artifact |
| `nlm artifact export [flags] <artifact-id>` | Export an artifact |
| `nlm artifact update [--name <name>] <artifact-id> [title]` | Rename artifact (new title from positional arg or --name) |
| `nlm artifact delete [flags] <artifact-id>` | Delete artifact |
| `nlm read-artifact <artifact-id>` | Print a text artifact |

### Guidebook

| Command | Description |
| --- | --- |
| `nlm guidebooks [flags]` | List all guidebooks |
| `nlm guidebook <guidebook-id>` | Get guidebook details |
| `nlm guidebook-details <guidebook-id>` | Get detailed guidebook info with sections and analytics |
| `nlm guidebook-publish <guidebook-id>` | Publish a guidebook |
| `nlm guidebook-share <guidebook-id>` | Share a guidebook |
| `nlm guidebook-ask <guidebook-id> <question>` | Ask a guidebook question |
| `nlm guidebook-rm <guidebook-id>` | Delete a guidebook |

### Generation

| Command | Description |
| --- | --- |
| `nlm generate-guide <notebook-id>` | Generate notebook guide |
| `nlm source-guide [flags] <notebook-id> [source-id...]` | Show the per-source auto-summary and keyword chips (cached on disk) |
| `nlm generate-chat [flags] <notebook-id> [prompt...]` | Stream a one-shot chat answer (use --conversation to follow up) |
| `nlm report-suggestions <notebook-id>` | Suggest report topics for notebook |
| `nlm audio-suggestions [flags] <notebook-id>` | Suggest audio-overview blueprints (emit JSON lines; pipe to create-audio) |
| `nlm generate-report [flags] <notebook-id>` | Generate multi-section report via chat (see --prompt, --sections) |

### Chat

| Command | Description |
| --- | --- |
| `nlm chat list [flags] [notebook-id]` | List chat sessions (server-side when a notebook is given) |
| `nlm chat history <notebook-id> <conversation-id>` | View conversation history |
| `nlm chat show [flags] <notebook-id> [conversation-id]` | Render a local chat transcript (see --citations) |
| `nlm chat delete [flags] <notebook-id>` | Delete server-side chat history |
| `nlm chat config <notebook-id> goal default \| <notebook-id> goal custom <prompt...> \| <notebook-id> length <default\|longer\|shorter>` | Configure chat settings |
| `nlm chat instructions set <notebook-id> "prompt"` | Set system instructions |
| `nlm chat instructions get <notebook-id>` | Show current system instructions |
| `nlm chat [flags] <notebook-id> [conversation-id \| prompt...]` | Open interactive chat (one-shot if a prompt is given; -f <file> reads a long prompt from file) |

### Research

| Command | Description |
| --- | --- |
| `nlm research [flags] <notebook-id> <query...>` | Run fast or deep research (JSON-lines by default; --md for markdown; --mode=fast\|deep) |

### Sharing

| Command | Description |
| --- | --- |
| `nlm share <notebook-id>` | Share notebook publicly |
| `nlm share-private <notebook-id>` | Share notebook privately |
| `nlm share-details <share-id>` | Get details of shared project |

### Other

| Command | Description |
| --- | --- |
| `nlm mcp` | Run the MCP server on stdin/stdout |
| `nlm auth [login] [options] [profile-name]` | Set up authentication from a browser profile |
| `nlm refresh` | Refresh stored authentication credentials |
| `nlm account [flags] [set <key> <value>]` | Show or update the authenticated user's NotebookLM account (ZwVcOc / hT54vc) |
<!-- END GENERATED COMMAND SIGNATURES -->

## Notebook

`notebook list` shows the first 10 notebooks on a TTY by default. When stdout
is piped, it emits the full list unless you pass `--limit`.

## Source

`source add` accepts URLs, file paths, literal text, or a sole `-`. A sole `-`
uploads stdin as one source; use `--name` to give piped text a useful title. To
add a list of URLs or paths from stdin, compose with `xargs`. Use `--replace`
to swap in a new upload, and `--mime` to override MIME detection for file
uploads.

`source sync` expands directories with tracked files by default. Add
`--include-untracked` to also include untracked, non-ignored files.

`source read --format=json` emits nlm's stable source projection:
`source_id`, `title`, and ordered `fragments`. Fragment fields are `start`,
`end`, `text`, `image_url`, `image_id`, `list_marker`, `bold`, `italic`,
`code`, `language`, `range_mismatch`, and `block_start`; zero-value optional
fields are omitted. `--format=raw` emits the decoded LoadSource protobuf with
protobuf field names. Its shape follows the wire model and is not a stable
scripting interface. The default format is `text`; `markdown` and `html`
produce presentation views.

## Note

Note bodies are sent verbatim as Markdown; the rich-text editor in the web
UI converts to Markdown on save, so piping a `.md` file in via stdin produces
the rendering you expect without any conversion step.

## Label

Labels are NotebookLM's source-clustering primitive. The autolabel suite
generates clusters; the manual suite lets you create, rename, and attach
labels yourself.

## Create

## Audio and Video

## Deck

`deck create` / `create-slides` accept `--format detailed|presenter` (detailed is the
default; presenter is experimental — its wire encoding is not yet HAR-verified) and the
standard source selectors (`--source-ids`, `--source-match`, `--source-exclude`,
`--label-ids`, `--label-match`, `--label-exclude`). When no selector is given, every
source in the notebook is used. Instructions are optional.

## Artifact

`artifact update` also accepts `--name` instead of a positional title.
`artifact export` selects server-rendered downloads by filename extension and
writes to stdout unless `--output` is provided. Type-4 flashcard apps also
accept `--format md|json|tsv|html`. It rejects non-READY artifacts and native
type-9 artifacts because no successful type-9 card-body payload has been
captured.

## Guidebook

## Generation

## Chat

For structured chat output, use `--citations=json`; add `--thinking` if you
also want reasoning events in the JSON-lines stream.

## Research

## Sharing

## Other
