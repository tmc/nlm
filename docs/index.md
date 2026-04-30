---
title: nlm
---
# nlm

An unofficial command-line interface and MCP server for [Google NotebookLM](https://notebooklm.google.com).

`nlm` lets you manage notebooks, sources, notes, audio/video overviews, and AI-generated content entirely from the terminal. It also exposes these capabilities as an [MCP](https://modelcontextprotocol.io) server so AI assistants like Claude can interact with NotebookLM directly.

## Highlights

- **Full notebook lifecycle** — create, list, delete notebooks; add URLs, PDFs, or plain text as sources
- **AI content generation** — summarize, critique, brainstorm, create study guides, FAQs, timelines, and more
- **Audio & video overviews** — generate and download podcast-style audio or video summaries
- **Interactive chat** — conversational sessions with full history, thinking traces, and citation tracking
- **MCP server** — expose NotebookLM to AI agents via `nlm mcp`
- **Scriptable** — pipe-friendly output, stdin support, batch workflows

## Quick start

```bash
# Install
go install github.com/tmc/nlm/cmd/nlm@latest

# Authenticate (opens browser)
nlm auth

# Create a notebook and add a source
nlm notebook create "Research Notes"
nlm source add NOTEBOOK_ID "https://example.com/paper.pdf"

# Chat with your sources
nlm chat NOTEBOOK_ID

# Generate an audio overview
nlm create-audio NOTEBOOK_ID "Focus on key findings"
```

## Next steps

- [Installation](installation.md) — build from source, requirements
- [Authentication](authentication.md) — how credentials work
- [Command reference](commands.md) — every command and flag
- [MCP server](mcp.md) — integrate with Claude, Cursor, and other AI tools
- [Examples](examples.md) — real-world workflows
