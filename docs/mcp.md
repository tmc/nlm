---
title: MCP Server
---
# MCP Server

nlm includes a [Model Context Protocol](https://modelcontextprotocol.io) server that exposes NotebookLM operations to AI assistants. Run it with:

```bash
nlm mcp
```

The server communicates over stdin/stdout using JSON-RPC, following the MCP specification.

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

If you've already run `nlm auth`, the credentials are stored in `~/.config/nlm/.env` and are loaded automatically — you can omit the `env` block.

## Available tools

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

### Note management

| Tool | Description | Mutating |
|------|-------------|----------|
| `list_notes` | List notes in a notebook | No |
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

### Chat

| Tool | Description | Mutating |
|------|-------------|----------|
| `generate_chat` | Free-form chat with notebook sources | Yes |

### Content generation

All generation tools accept `notebook_id` and `source_ids` parameters:

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

## Tool annotations

Each tool is annotated with MCP hints:

- **Read-only** tools (list operations) are marked `readOnlyHint: true`
- **Mutating** tools (create, update) are marked `destructiveHint: false`
- **Destructive** tools (delete) are marked `destructiveHint: true`
- All tools are marked `openWorldHint: false` (closed system)

## Pagination

List tools support pagination via `limit` (default 50, max 100) and `offset` parameters. Responses include `total`, `returned`, `has_more`, and `next_offset` fields.
