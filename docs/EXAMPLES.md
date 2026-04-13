---
title: Examples
---
# Examples

Practical workflows for common nlm use cases.

## Basic workflow

```bash
# Authenticate
nlm auth

# Create a notebook
nlm create "Project Research"
# => Created notebook abc123

# Add sources
nlm add abc123 "https://arxiv.org/abs/2103.00020"
nlm add abc123 ./requirements.pdf
nlm add abc123 "Key decisions from the planning meeting on March 5th"

# List what's there
nlm sources abc123

# Chat with your sources
nlm chat abc123

# Generate an audio summary
nlm audio-create abc123 "Conversational, focus on key findings"
```

## Adding sources from stdin

Pipe content directly into a notebook:

```bash
# From a command
curl -s https://api.example.com/data.json | nlm add NOTEBOOK_ID - --mime application/json

# From a file with explicit MIME type
cat report.xml | nlm add NOTEBOOK_ID - --mime text/xml

# Quick text note
echo "Remember to follow up on the API changes" | nlm add NOTEBOOK_ID -
```

## Batch source processing

```bash
# Add all PDFs in a directory
for f in papers/*.pdf; do
    nlm add NOTEBOOK_ID "$f"
    sleep 1  # rate limiting
done

# Process each source with a transformation
nlm sources NOTEBOOK_ID | while read -r id rest; do
    echo "=== Summarizing: $rest ==="
    nlm summarize NOTEBOOK_ID "$id"
done
```

## Content generation pipeline

Generate multiple output formats from a single notebook:

```bash
NB=abc123

# Structured outputs
nlm generate-guide "$NB" > guide.md
nlm study-guide "$NB" SOURCE_ID > study-guide.md
nlm faq "$NB" SOURCE_ID > faq.md
nlm timeline "$NB" SOURCE_ID > timeline.md
nlm briefing-doc "$NB" SOURCE_ID > briefing.md

# Audio overview
nlm audio-create "$NB" "Professional summary"
# Wait for generation, then download
nlm --direct-rpc audio-download "$NB" overview.mp3
```

## Interactive chat with history

```bash
# Start a new chat session
nlm chat NOTEBOOK_ID
# Type questions interactively, Ctrl+D to exit

# Resume a previous session
nlm chat-list NOTEBOOK_ID
nlm chat NOTEBOOK_ID CONVERSATION_ID

# Show reasoning and citations
nlm chat --thinking --history NOTEBOOK_ID

# One-shot question (no interactive session)
nlm chat NOTEBOOK_ID "What are the three main conclusions?"
```

## Research workflow

```bash
# Create a research notebook
NB=$(nlm create "Literature Review" 2>&1 | grep -o '[a-f0-9-]\{36\}')

# Add papers
nlm add "$NB" paper1.pdf
nlm add "$NB" paper2.pdf
nlm add "$NB" "https://arxiv.org/abs/2005.14165"

# Get source IDs
SOURCES=$(nlm sources "$NB" | awk '{print $1}' | tr '\n' ' ')

# Generate analysis
nlm summarize "$NB" $SOURCES
nlm critique "$NB" $SOURCES
nlm mindmap "$NB" $SOURCES

# Deep research
nlm research "$NB" "What are the methodological gaps across these papers?"

# Audio for review
nlm audio-create "$NB" "Academic tone, compare methodologies"
```

## Scripting with confirmation bypass

Use `-y` to skip confirmation prompts in scripts:

```bash
#!/bin/bash
# Cleanup old notebooks
nlm ls | grep "2024-Q1" | awk '{print $1}' | while read id; do
    nlm -y rm "$id"
done
```

## MCP integration

Use nlm as an MCP server so AI assistants can interact with NotebookLM:

```bash
# Test the MCP server manually
echo '{"jsonrpc":"2.0","id":1,"method":"tools/list"}' | nlm mcp

# Configure in Claude Desktop (see mcp.md for full setup)
```

## Debugging

```bash
# Enable debug output
nlm --debug list
nlm --debug sources NOTEBOOK_ID

# Inspect raw protocol details
nlm --debug-dump-payload sources NOTEBOOK_ID
nlm --debug-parsing chat NOTEBOOK_ID "test"
nlm --debug-field-mapping sources NOTEBOOK_ID

# Heartbeat check
nlm hb
```
