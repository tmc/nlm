# nlm Command Reference

This is a compact map of the current CLI surface. Prefer `nlm --help` and
`nlm <command> --help` when exact flags matter.

## Notebooks

```bash
nlm notebook list [--limit N|--all]     # List notebooks
nlm notebook create <title>             # Create notebook
nlm -y notebook delete <id>             # Delete notebook
nlm notebook featured                   # List featured notebooks
```

## Sources

```bash
nlm source list <notebook-id>                         # List sources
nlm source add [--name NAME] [--mime TYPE] <notebook-id> <source|-> [source...]
nlm source sync [flags] <notebook-id> [paths...]      # Managed txtar sync
nlm source pack [--chunk N] [paths...]                # Preview sync payload
nlm source delete <notebook-id> <source-id|-|a,b,c>   # Delete sources
nlm source rename <source-id> <new-name>              # Rename source
nlm source refresh <notebook-id> <source-id>          # Refresh source
nlm source check <source-id> [notebook-id]            # Check freshness
nlm source read <source-id> [notebook-id]             # Print indexed text
```

Useful `source sync` flags:

```bash
--name, -n <name>   Override generated source title
--force             Re-upload unchanged content
--dry-run           Show changes without uploading
--max-bytes <n>     Chunk threshold
--json              Emit NDJSON progress
```

## Notes

```bash
nlm note list <notebook-id>                         # List notes
nlm note read <notebook-id> <note-id>               # Read note
nlm note create <notebook-id> <title> [content]     # Create note
nlm note update <notebook-id> <note-id> <content> <title>
nlm note delete <notebook-id> <note-id>             # Delete note
```

## Content Creation

```bash
nlm create-audio <notebook-id> <instructions>       # Create audio overview
nlm create-video <notebook-id> <instructions>       # Create video overview
nlm create-slides <notebook-id> <instructions>      # Create slide deck
nlm create-report <notebook-id> <type> [desc] [instructions]
nlm report-suggestions <notebook-id>                # Valid report topics/types
nlm audio-suggestions <notebook-id>                 # Audio blueprint JSON lines
```

## Audio And Video

```bash
nlm audio-list <notebook-id>             # List audio overviews
nlm audio-get <notebook-id>              # Get audio details
nlm --direct-rpc audio-download <notebook-id> [file]
nlm audio-rm <notebook-id>               # Delete audio overview
nlm audio-share <notebook-id>            # Share audio overview

nlm video-list <notebook-id>             # List video overviews
nlm video-get <notebook-id>              # Get video details
nlm --direct-rpc video-download <notebook-id> [file]
```

## Artifacts

```bash
nlm artifact list <notebook-id>                    # List artifacts
nlm artifact get <artifact-id>                     # Get artifact details
nlm artifact update <artifact-id> [new-title]      # Rename artifact
nlm artifact delete <artifact-id>                  # Delete artifact
```

## Chat And Generation

```bash
nlm chat <notebook-id> [conversation-id|prompt]     # Interactive or one-shot chat
nlm generate-chat [flags] <notebook-id> <prompt>    # Streaming one-shot chat
nlm chat list [notebook-id]                         # List conversations
nlm chat history <notebook-id> <conversation-id>    # Server-side history
nlm chat show <notebook-id> <conversation-id>       # Local transcript render
nlm chat delete <notebook-id>                       # Delete chat history
nlm chat config <notebook-id> <setting> [value]     # Configure chat
nlm chat instructions set <notebook-id> "prompt"    # Set instructions
nlm chat instructions get <notebook-id>             # Show instructions
nlm generate-guide <notebook-id>                    # Generate notebook guide
nlm source-guide <notebook-id> [source-id...]       # Per-source summaries
nlm generate-report [flags] <notebook-id>           # Multi-section report via chat
```

Useful chat and generation flags:

```bash
--conversation, -c <id>  Continue a conversation
--web                    Use the latest server-side conversation
--source-ids <ids>       Focus on source IDs, comma-separated or stdin with -
--source-match <regex>   Focus on source titles or UUIDs matching regex
--citations <mode>       off|block|stream|tail|overlay|json
--thinking               Show thinking headers
--thinking-jsonl         Emit thinking/answer/citation JSON lines
--verbose, -v            Show full thinking traces
```

## Content Transforms

All take `<notebook-id>` and source selectors. Most accept optional source
IDs; `mindmap` requires at least one source ID. Most also support source focus
flags such as `--source-ids` and `--source-match`.

```bash
nlm summarize
nlm explain
nlm rephrase
nlm expand
nlm critique
nlm brainstorm
nlm verify
nlm outline
nlm study-guide
nlm faq
nlm briefing-doc
nlm mindmap
nlm timeline
nlm toc
```

## Research

```bash
nlm research [--mode fast|deep] [--md] [--import] <notebook-id> "query"
```

Useful research flags:

```bash
--mode <fast|deep>   Research mode, default deep
--md                 Emit raw markdown instead of JSON-lines events
--poll-ms <n>        Override deep-research polling interval
--import             Import discovered sources after completion
```

## Guidebooks

```bash
nlm guidebooks
nlm guidebook <guidebook-id>
nlm guidebook-details <guidebook-id>
nlm guidebook-publish <guidebook-id>
nlm guidebook-share <guidebook-id>
nlm guidebook-ask <guidebook-id> <question>
nlm guidebook-rm <guidebook-id>
```

## Sharing And System

```bash
nlm share <notebook-id>
nlm share-private <notebook-id>
nlm share-details <share-id>
nlm auth [profile]
nlm auth --authuser N
nlm refresh
nlm mcp
nlm feedback <message>
```

## Common Global Flags

| Flag | Description |
|------|-------------|
| `-y` | Skip confirmation prompts |
| `--debug` | Send request/response diagnostics to stderr |
| `--direct-rpc` | Use direct RPC path, required for media downloads |
| `--authuser N` | Select Google account profile |
| `--auth PATH` | Use explicit auth file |
| `--cookies PATH` | Use explicit cookies file |
| `--experimental` | Enable experimental commands or behavior |

## Compatibility Aliases

These still run, but new guidance should use the canonical noun-first forms:

```bash
nlm list, nlm ls                    # notebook list
nlm create                          # notebook create
nlm rm                              # notebook delete
nlm sources                         # source list
nlm add                             # source add
nlm sync                            # source sync
nlm sync-pack                       # source pack
nlm rm-source, nlm source-rm        # source delete
nlm rename-source                   # source rename
nlm refresh-source                  # source refresh
nlm check-source                    # source check
nlm notes                           # note list
nlm read-note                       # note read
nlm new-note                        # note create
nlm update-note                     # note update
nlm rm-note                         # note delete
nlm artifacts                       # artifact list
nlm get-artifact                    # artifact get
nlm rename-artifact                 # artifact update
nlm delete-artifact                 # artifact delete
nlm chat-list                       # chat list
nlm delete-chat                     # chat delete
nlm chat-config                     # chat config
nlm set-instructions                # chat instructions set
nlm get-instructions                # chat instructions get
```
