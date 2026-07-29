# nlm

[![Go Reference](https://pkg.go.dev/badge/github.com/tmc/nlm.svg)](https://pkg.go.dev/github.com/tmc/nlm)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

> **nlm** is a single-static-binary [NotebookLM](https://notebooklm.google.com)
> client for people who live in the terminal and in CI. No Python venv, no
> runtime — one Go binary that is at once a scriptable/pipeable CLI, an
> interactive streaming chat client, and an MCP server for AI agents.
> Reverse-engineered wire protocol, lossless-verified.

`nlm` combines compiled-protobuf wire modeling and lossless capture verification
with source selection by name, label, or regular expression, directory-tree
sync, rich-note rendering, and citations that resolve back to local file and
line numbers. A static binary by itself is not unique in this field; this
combination is. The project deliberately keeps a focused, composable surface
rather than chasing every possible feature.

## Feature support

| Feature | Notes |
|---|---|
| Browser authentication | `nlm auth login` extracts credentials from an already signed-in Chrome, Brave, or Edge profile — no DevTools copy-paste |
| Interactive & scriptable chat | Streaming `nlm chat` REPL with persistent sessions and slash commands (`/history`, `/new`, `/fork`, `/file`); pass a prompt for one-shot/script use |
| Source selection by name, label, or regex | `--source-match`, `--source-exclude`, `--label-match`, `--label-exclude` |
| Sources: files, URLs, text, stdin | `nlm source add`; PDFs upload via Google's resumable protocol |
| Local-tree sync | Idempotent SHA-256 directory sync with `.nlmignore`, exclude patterns, and chunking |
| Rich note rendering | RichDocument notes to Markdown or HTML, with citation excerpts |
| Labels & autolabel | Hand-curated labels plus generated autolabels |
| Audio, video & slide generation | `audio create`, `video create`, `deck create`, plus report generation |
| Deep research | Event stream or self-contained Markdown (`nlm research --md`) |
| Artifacts | List, get, rename, delete, share, and export/download rendered artifacts |
| Flashcard export | Type-4 flashcard artifacts → Markdown, JSON, TSV, or HTML |
| Sharing | Public and private share links |
| MCP server | Built-in stdio server: notebook, source, note, chat, artifact, generation, and research tools |
| Citation → `file:line` | Resolves citations back into txtar-bundled local sources |

**Not yet:** mind-map export and native type-9 flashcard export — both still need capture-backed wire modeling.

## Quickstart

```bash
go install github.com/tmc/nlm/cmd/nlm@latest
nlm auth login
nlm notebook list
nlm chat <notebook-id> "summarize the key findings"
```

`go install` needs the Go toolchain (1.25+). Prebuilt release binaries and a
Homebrew formula (`brew install tmc/tap/nlm`) are planned so the single binary
can be fetched without Go.

## Go package

The high-level client is importable as `github.com/tmc/nlm/notebooklm`:

```go
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/tmc/nlm/notebooklm"
)

func main() {
	client := notebooklm.New(notebooklm.Credentials{
		AuthToken: os.Getenv("NLM_AUTH_TOKEN"),
		Cookies:   os.Getenv("NLM_COOKIES"),
	})
	notebooks, err := client.ListRecentlyViewedProjects(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(len(notebooks))
}
```

The package exposes notebook, source, chat, note, research, and artifact
operations. It accepts explicit credentials and does not launch a browser;
applications remain responsible for acquiring and refreshing their session.
NotebookLM does not publish a supported service API, so the package provides a
documented Go surface over a best-effort private-RPC implementation.

## Authentication

`nlm auth login` uses chromedp to launch Chrome, Brave, or Edge headlessly,
opens NotebookLM, extracts the `SNlM0e` token and browser cookies, and stores
them in `~/.nlm/env`. It requires a browser profile that is already signed into
Google. It does **not** perform unattended SSO on a fresh machine with no
signed-in browser session.

```bash
nlm auth login
nlm auth login -profile Work
nlm auth login -all -notebooks
nlm auth login -authuser 2
```

Use `-keep-open N -debug` when the selected profile needs attention: the
browser stays open for `N` seconds after credential extraction. Connect to an
already running browser instead with `-cdp-url`:

```bash
nlm auth login -keep-open 30 -debug
nlm auth login -cdp-url ws://localhost:9222
```

`-profile` selects a named browser profile. `-all` probes all available
profiles, and `-authuser N` selects the Google account index within a
multi-account session. Commands also accept `--authuser N`, or you can export
`NLM_AUTHUSER=N`.

For CI and scripting, harvest credentials on a signed-in machine and export
them explicitly:

```bash
nlm auth --print-env
nlm --cookies 'SID=...; HSID=...; SSID=...' notebook list
nlm --auth <auth-token> notebook list
```

The CLI retries authentication-shaped failures by re-harvesting the cached
profile. Set `NLM_AUTO_REFRESH=false` to disable that retry. This is reactive
recovery, not unattended SSO, and it cannot repair a browser profile whose
Google session has expired.

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
cat notes.md | nlm source add --name "Meeting notes" <notebook-id> -
printf '%s\n' https://example.com/a ./notes.pdf |
    xargs -n 1 nlm source add <notebook-id>
nlm source sync <notebook-id> .
nlm source pack .
nlm source read <source-id> [notebook-id]
nlm source read --format=markdown <source-id> [notebook-id]
nlm source delete <notebook-id> <source-id>
```

When you pass `-` to `source add`, all of stdin becomes one source. To add a
list of URLs or paths, compose with `xargs` as shown above.

`source read` supports `--format=text|markdown|html|json|raw`. The `json`
format is a stable object with `source_id`, `title`, and ordered `fragments`;
each fragment records its offsets and decoded text, image, list, style, and
code fields. The `raw` format is the unstable decoded LoadSource protobuf,
emitted with protobuf field names for debugging.

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
# Generate studio content (noun-first; create-audio/-video/-slides aliases also work)
nlm audio create <notebook-id> "deep dive on topic X"
nlm video create <notebook-id> "whiteboard walkthrough"
nlm deck create <notebook-id> "presentation summary"
nlm report-suggestions <notebook-id>
nlm create-report <notebook-id> <report-type> "focused brief"

nlm artifact list <notebook-id>
nlm artifact get <artifact-id>
nlm artifact export <artifact-id> --format md --output artifact.md
nlm artifact update <artifact-id> "New Title"
nlm artifact delete <artifact-id>

nlm audio list <notebook-id>
nlm audio get <notebook-id>
nlm audio share <notebook-id>
nlm audio delete <notebook-id>
nlm --direct-rpc audio download <notebook-id> overview.mp3

nlm deck download <notebook-id> --id <artifact-id> --format pptx --output deck.pptx
```

If generated audio or slide deck output cannot be fetched directly from the CLI,
the download command prints the NotebookLM browser URL so you can download it
from the web UI.

`artifact export` downloads READY artifacts that expose a server-rendered file,
selected by its filename extension. Google-AI-mode type-4 flashcard artifacts
also support generated Markdown, JSON, tab-separated, and self-contained HTML.
Native type-9 flashcard artifacts remain unsupported until a successful deck
payload is captured.

### Chat

```bash
nlm chat <notebook-id>
nlm chat <notebook-id> "What is this about?"
nlm chat <notebook-id> --source-ids s1,s2 "..."
nlm chat <notebook-id> --source-ids - "..." < ids.txt
nlm chat <notebook-id> --source-match '^design/' --source-exclude 'draft' "..."
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
`source_discovered`, `report_chunk`, and `complete`. `--md` switches to a
self-contained Markdown report, rewriting NotebookLM citation markers such as
`[cite: 1, 2]` into footnotes linked to the discovered source URLs.

### Other

```bash
nlm auth login
nlm refresh
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

The server includes notebook, source, note, artifact, chat, generation, and
deep-research tools. See [docs/mcp.md](docs/mcp.md) for the complete tool
surface and input/output behavior.

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
| 3 | auth | Auth required / auth expired | `nlm auth login` and retry |
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
--citations mode     Citation rendering: off|list|json (default list)
--thinking           Show reasoning traces while streaming chat output
--prompt-file path   Read a one-shot chat prompt from a file
--mode string        Research mode: fast or deep
--md                 Emit Markdown with source footnotes (research)
-y, --yes            Skip confirmation prompts
```

## License

MIT — see [LICENSE](LICENSE).
