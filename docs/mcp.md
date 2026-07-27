---
title: MCP Server
---
# MCP Server

nlm includes a [Model Context Protocol](https://modelcontextprotocol.io) server that exposes NotebookLM operations to AI assistants. Run it with:

```bash
nlm mcp
```

The server communicates over stdin/stdout using JSON-RPC. It exposes 38 tools:
24 direct notebook operations and 14 content-generation tools.

## Client configuration

### Claude Desktop

Add to `~/Library/Application Support/Claude/claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "nlm": {
      "command": "nlm",
      "args": ["mcp"],
      "env": {
        "NLM_AUTH_TOKEN": "your-token",
        "NLM_COOKIES": "your-cookies"
      }
    }
  }
}
```

### Claude Code

Add to your project's `.claude/settings.json` or global settings:

```json
{
  "mcpServers": {
    "nlm": {
      "command": "nlm",
      "args": ["mcp"],
      "env": {
        "NLM_AUTH_TOKEN": "your-token",
        "NLM_COOKIES": "your-cookies"
      }
    }
  }
}
```

### Cursor

Add to your Cursor MCP settings:

```json
{
  "nlm": {
    "command": "nlm",
    "args": ["mcp"],
    "env": {
      "NLM_AUTH_TOKEN": "your-token",
      "NLM_COOKIES": "your-cookies"
    }
  }
}
```

If you have already run `nlm auth login`, credentials are stored in
`~/.nlm/env` and loaded automatically, so you can omit the `env` block. Browser
login requires a Chrome, Brave, or Edge profile already signed into Google.

## Available tools

The names below are the exact names returned by MCP `tools/list`.

### Notebook management

| Tool | Description | Mutating |
|------|-------------|----------|
| `list_notebooks` | List notebooks with pagination | No |
| `create_notebook` | Create a new notebook | Yes |
| `delete_notebook` | Delete a notebook | Destructive |

### Source management

| Tool | Description | Mutating |
|------|-------------|----------|
| `list_sources` | List sources in a notebook | No |
| `add_source_text` | Add text content as a source | Yes |
| `add_source_url` | Add a URL as a source | Yes |
| `delete_source` | Remove a source | Destructive |

`add_source_text` is the context-injection primitive for agents. The MCP server
does not read local file paths; the caller reads a file or diff and supplies its
contents as `content`, with a source `title`.

### Note management

| Tool | Description | Mutating |
|------|-------------|----------|
| `list_notes` | List notes in a notebook | No |
| `read_note` | Read a note's title and plain-text projection | No |
| `create_note` | Create a new note | Yes |
| `delete_note` | Delete a note | Destructive |

### Artifacts

| Tool | Description | Mutating |
|------|-------------|----------|
| `list_artifacts` | List artifacts in a notebook | No |
| `rename_artifact` | Rename an artifact | Yes |

### Audio

| Tool | Description | Mutating |
|------|-------------|----------|
| `create_audio_overview` | Generate an audio overview | Yes |
| `get_audio_overview` | Get audio overview status | No |
| `share_audio` | Share an audio overview | Yes |

### Video, slides, and app artifacts

| Tool | Description | Mutating |
|------|-------------|----------|
| `create_video_overview` | Generate a video overview | Yes |
| `create_slide_deck` | Generate a slide deck | Yes |
| `create_app_artifact` | Generate a prototype, mind map, or canvas app artifact | Yes |

### Chat and instructions

| Tool | Description | Mutating |
|------|-------------|----------|
| `generate_chat` | Free-form chat with notebook sources | Yes |
| `get_instructions` | Read the notebook's custom chat instructions | No |
| `set_instructions` | Replace the notebook's custom chat instructions | Yes |

### Deep research

| Tool | Description | Mutating |
|------|-------------|----------|
| `start_deep_research` | Start deep research and return a research ID | Yes |
| `poll_deep_research` | Poll a research ID; returns `done=true` with the completed content | No |

Deep research is asynchronous. Call `start_deep_research` once, retain the
returned research ID, then call `poll_deep_research` until `done` is true. The
current stdio server does not emit progress notifications or provide a blocking
watch tool.

### Content generation

All 14 generation tools accept `notebook_id` and optional `source_ids`
parameters:

| Tool | Description |
|------|-------------|
| `generate_summarize` | Summarize source content |
| `generate_faq` | Generate FAQ |
| `generate_study_guide` | Generate a study guide |
| `generate_briefing_doc` | Create a briefing document |
| `generate_timeline` | Create a timeline |
| `generate_toc` | Generate table of contents |
| `generate_mindmap` | Generate an interactive mind map |
| `generate_outline` | Create a structured outline |
| `generate_rephrase` | Rephrase content |
| `generate_expand` | Expand on content |
| `generate_critique` | Critical analysis |
| `generate_brainstorm` | Brainstorm ideas |
| `generate_verify` | Verify facts |
| `generate_explain` | Explain concepts |

## Common agent workflows

### Inject local text and ask about it

1. Call `add_source_text` with the notebook ID, a descriptive title, and the
   text to inject.
2. Retain the returned source ID.
3. Call `generate_chat` with the notebook ID and a question.

Use `create_note` instead when the content should be an editable notebook note
rather than a grounded source.

### Run deep research

1. Call `start_deep_research` with `notebook_id` and `query`.
2. Read `research_id` from the result.
3. Call `poll_deep_research` with the notebook and research IDs until the
   result reports completion.

### Generate from selected sources

Pass source UUIDs through `source_ids` to any `generate_*` tool. The CLI's
name/label/regex selectors are not part of the MCP input schema; resolve the
desired UUIDs with `list_sources` first.

## Tool annotations

Each tool is annotated with MCP hints:

- **Read-only** tools (list operations) are marked `readOnlyHint: true`
- **Mutating** tools (create, update) are marked `destructiveHint: false`
- **Destructive** tools (delete) are marked `destructiveHint: true`
- All tools are marked `openWorldHint: false` (closed system)

## Pagination

List tools support pagination via `limit` (default 50, max 100) and `offset` parameters. Responses include `total`, `returned`, `has_more`, and `next_offset` fields.

The paginated list tools are `list_notebooks`, `list_sources`, `list_notes`,
and `list_artifacts`.
