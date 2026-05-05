# nlm

Command-line interface and MCP server for [Google NotebookLM](https://notebooklm.google.com).

## Quickstart

```bash
go install github.com/tmc/nlm/cmd/nlm@latest
nlm auth
nlm notebook list
nlm chat <notebook-id> "summarize the key findings"
```

## Authentication

`nlm auth` finds a logged-in browser profile (Chrome, Brave, Edge) and caches
credentials under `~/.nlm`. For CI and scripting, export credentials directly:

```bash
nlm auth --print-env
nlm --cookies 'SID=...; HSID=...; SSID=...' notebook list
nlm --auth <auth-token> notebook list
```

For multi-account setups, use `nlm auth --authuser N` or export `NLM_AUTHUSER=N`.

## Usage

```bash
nlm <command> [arguments]
```

Canonical help uses grouped noun-first commands for notebooks, sources, notes,
artifacts, and chat administration:

```bash
nlm notebook list
nlm source add <notebook-id> https://example.com
nlm note create <notebook-id> "Title" "Body"
nlm artifact list <notebook-id>
nlm chat list <notebook-id>
```

Legacy top-level aliases still work for existing scripts, but new usage should
prefer the grouped commands above.

### Notebook

```bash
nlm notebook list
nlm notebook list --limit 25
nlm notebook list --all
nlm notebook create "My Notebook"
nlm notebook delete <notebook-id>
nlm notebook featured
nlm notebook rename <notebook-id> "New Title"
nlm notebook emoji <notebook-id> "📚"
nlm notebook description <notebook-id> "Creator notes"
nlm notebook cover <notebook-id> <preset-id>
nlm notebook cover-image <notebook-id> ./cover.png
nlm notebook unrecent <notebook-id>
```

`notebook list` shows the first 10 notebooks on a TTY by default. Use `--limit`
to choose a different cap, or `--all` to suppress the TTY cap entirely.

### Source

```bash
nlm source list <notebook-id>
nlm source add <notebook-id> https://example.com/article
nlm source add <notebook-id> ./document.pdf
nlm source add <notebook-id> "Meeting notes from March 5"
printf '%s\n' https://example.com/a ./notes.pdf | nlm source add <notebook-id> -
nlm source sync <notebook-id> .
nlm source pack .
nlm source read <source-id> [notebook-id]
nlm source delete <notebook-id> <source-id>
```

When you pass `-` to `source add`, stdin is treated as one source reference per
line. For multi-line text, pass a file path or use notes instead.

### Note

```bash
nlm note list <notebook-id>
nlm note read <notebook-id> <note-id>
nlm note create <notebook-id> "Title" "Content"
nlm note create <notebook-id> "Title" < content.md
nlm note update <notebook-id> <note-id> "Content" "Title"
nlm note delete <notebook-id> <note-id>
```

Note bodies are sent verbatim as Markdown — pipe a `.md` file through stdin
and you get the rendering you expect, no HTML conversion needed.

### Label

```bash
nlm label list <notebook-id>
nlm label generate <notebook-id>
nlm label create <notebook-id> "Name" [emoji]
nlm label rename <notebook-id> <label-id> "New Name"
nlm label emoji <notebook-id> <label-id> "🏷️"
nlm label delete <notebook-id> <label-id> [<label-id>...]
nlm label attach <notebook-id> <label-id> <source-id>
nlm label unlabeled <notebook-id>
nlm label relabel-all <notebook-id>
```

`label list` and `label generate` cover the autolabel suite; `label create`
and friends are the manual surface for hand-curated labels.

### Create, Artifact, Audio, and Video

```bash
nlm create-audio <notebook-id> "deep dive on topic X"
nlm create-video <notebook-id> "whiteboard walkthrough"
nlm create-slides <notebook-id> "presentation summary"
nlm report-suggestions <notebook-id>
nlm create-report <notebook-id> <report-type> "focused brief"

nlm artifact list <notebook-id>
nlm artifact get <artifact-id>
nlm artifact update <artifact-id> "New Title"
nlm artifact delete <artifact-id>

nlm audio list <notebook-id>
nlm audio get <notebook-id>
nlm audio share <notebook-id>
nlm audio delete <notebook-id>
nlm --direct-rpc audio download <notebook-id> overview.mp3
nlm video list <notebook-id>
nlm video get <notebook-id>
nlm --direct-rpc video download <notebook-id> overview.mp4
```

### Chat

```bash
nlm chat <notebook-id>
nlm chat <notebook-id> "What is this about?"
nlm chat <notebook-id> --source-ids s1,s2 "..."
nlm chat <notebook-id> --source-ids - "..." < ids.txt
nlm chat list
nlm chat list <notebook-id>
nlm chat history <notebook-id> <conversation-id>
nlm chat show <notebook-id> <conversation-id>
nlm chat delete <notebook-id>
nlm chat config <notebook-id> <setting> [value]
nlm chat instructions set <notebook-id> "Always cite sources and be concise"
nlm chat instructions get <notebook-id>
nlm generate-chat <notebook-id> "summarize"
```

Under `--citations=json`, the chat stream emits JSON-lines events on stdout.
Add `--thinking` to include reasoning traces:
`{"phase":"thinking","text":...}`, `{"phase":"answer","text":...}`,
`{"phase":"citation","index":...,"source_id":...,"confidence":...}`,
`{"phase":"followup","text":...}`, `{"phase":"done"}`.

### Content Transformation

```bash
nlm summarize <notebook-id>     nlm critique <notebook-id>
nlm rephrase <notebook-id>      nlm brainstorm <notebook-id>
nlm expand <notebook-id>        nlm verify <notebook-id>
nlm explain <notebook-id>       nlm outline <notebook-id>
nlm study-guide <notebook-id>   nlm faq <notebook-id>
nlm timeline <notebook-id>      nlm toc <notebook-id>
nlm briefing-doc <notebook-id>  nlm mindmap <notebook-id> <source-id>
```

Pass source IDs positionally, or use `--source-ids` / `--source-match` to
scope the command without listing them on the command line.

### Research and Sharing

```bash
nlm research <notebook-id> "research query"
nlm research <notebook-id> --mode=fast "query"
nlm research <notebook-id> --md "query" > report.md
nlm research <notebook-id> "query" | jq -r \
    'select(.type=="source_discovered") | .url' \
    | nlm source add <notebook-id> -

nlm share <notebook-id>
nlm share-private <notebook-id>
nlm share-details <share-id>
```

The default research event stream uses `type` values such as `progress`,
`source_discovered`, `report_chunk`, and `complete`. `--md` switches to raw
markdown output.

### Other

```bash
nlm auth
nlm refresh
nlm feedback "Great tool!"
nlm mcp
```

## MCP Server

`nlm` includes a built-in [Model Context Protocol](https://modelcontextprotocol.io/)
server that exposes NotebookLM operations as tools for AI assistants.

```bash
nlm mcp
```

Configure it in your MCP client:

```json
{
  "mcpServers": {
    "nlm": {
      "command": "nlm",
      "args": ["mcp"]
    }
  }
}
```

Available MCP tools include `list_notebooks`, `list_sources`, `list_notes`,
`list_artifacts`, `create_notebook`, `create_note`, `create_audio_overview`,
`create_video_overview`, `create_slide_deck`, `generate_chat`, `read_note`,
`set_instructions`, `get_instructions`, `start_deep_research`, and
`poll_deep_research`.

## Composing with the shell

`nlm` is designed to be pipeline-friendly:

- List commands auto-detect TTY. At a terminal they align columns; when piped
  they emit literal tabs so `cut`, `awk`, and `paste` work cleanly.
- `--json` on list commands emits one JSON object per record on stdout.
- `-` means "read from stdin, one value per line" for commands such as
  `source add`, `source delete`, and `chat --source-ids -`.
- Destructive commands refuse to prompt when stdin is not a TTY. Pass `-y` to
  bypass prompts in scripts.

Worked examples live in `docs/EXAMPLES.md`.

## Exit codes

Shell scripts can branch on `nlm`'s exit status. Each non-zero code also
prints `nlm: exit-class=<name> (exit N)` to stderr:

| Code | Class | Meaning | Typical handling |
|------|-------|---------|------------------|
| 0 | success | Ran to completion | continue |
| 1 | generic | Unclassified error | inspect stderr |
| 2 | bad-args | Bad invocation (missing arg, unknown flag) | fix the command |
| 3 | auth | Auth required / auth expired | `nlm auth` and retry |
| 4 | not-found | Notebook / source / artifact does not exist | stop; target is wrong |
| 5 | precondition | Permanent precondition (source-cap, quota, deleted) | stop; retry will not help |
| 6 | transient | Rate-limit, 5xx, network | retry with backoff |
| 7 | busy | Resource still generating / polling incomplete | sleep and poll |

## Selected Flags

Run `nlm <command> -h` for per-command usage. Common flags:

```text
--version            Print version and exit
--auth string        Auth token
--cookies string     Browser cookies (SID, HSID, SSID)
--profile string     Chrome profile to use
--debug              Enable debug output
--json               Emit output as JSON / JSON-lines
--direct-rpc         Use direct RPC calls for audio/video operations
--experimental       Enable experimental commands
--mime string        Override MIME type for source uploads
--name string        Override source or artifact title
--replace string     Replace an existing source when adding
--source-ids string  Restrict chat/report/transform commands to source IDs
--source-match regex Restrict chat/report/transform commands by source title or ID
--citations mode     Citation rendering: off|block|stream|tail|overlay|json
--thinking           Show reasoning traces while streaming chat output
--prompt-file path   Read a one-shot chat prompt from a file
--mode string        Research mode: fast or deep
--md                 Emit raw markdown instead of JSON-lines (research)
-y, --yes            Skip confirmation prompts
```

## Package Structure

```text
cmd/nlm/                    CLI entry point
internal/
  notebooklm/api/           High-level API client
  notebooklm/rpc/           Low-level RPC client
  batchexecute/             Google batchexecute protocol
  beprotojson/              Proto <-> batchexecute JSON marshaling
  nlmmcp/                   MCP server implementation
  auth/                     Browser cookie extraction
gen/
  method/                   RPC argument encoders
  service/                  Generated service clients
  notebooklm/v1alpha1/      Protocol buffer definitions
```

## License

MIT
