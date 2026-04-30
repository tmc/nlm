---
title: Examples
---
# Examples

Practical workflows for common `nlm` use cases.

## Basic workflow

```bash
# Authenticate
nlm auth

# Create a notebook
NB=$(nlm notebook create "Project Research")

# Add sources
nlm source add "$NB" "https://arxiv.org/abs/2103.00020"
nlm source add "$NB" ./requirements.pdf
nlm source add "$NB" "Key decisions from the planning meeting on March 5"

# List what's there
nlm source list "$NB"

# Chat with your sources
nlm chat "$NB"

# Generate an audio summary
nlm create-audio "$NB" "Conversational, focus on key findings"
```

## Listing notebooks

```bash
# Default TTY view shows the first 10 notebooks
nlm notebook list

# Show a larger slice
nlm notebook list --limit 25

# Show everything on a TTY
nlm notebook list --all
```

## Adding sources from stdin

`source add ... -` reads one source reference per line.

```bash
# Mix URLs and file paths in one batch
printf '%s\n' \
  "https://example.com/spec" \
  "./docs/brief.pdf" \
  | nlm source add NOTEBOOK_ID -

# Add every PDF found under a directory
find papers -name '*.pdf' -print | nlm source add NOTEBOOK_ID -

# For free-form text, pass a quoted argument instead of stdin batch mode
nlm source add NOTEBOOK_ID "Remember to follow up on the API changes"
```

## Working with notes

```bash
nlm note create NOTEBOOK_ID "Open questions" "Compare Q3 and Q4 assumptions"
nlm note list NOTEBOOK_ID
nlm note read NOTEBOOK_ID NOTE_ID

# Multi-line content from stdin
nlm note create NOTEBOOK_ID "Draft outline" < outline.md
```

## Batch source processing

```bash
nlm source list NOTEBOOK_ID | while read -r id rest; do
    echo "=== Summarizing: $rest ==="
    nlm summarize NOTEBOOK_ID "$id"
done
```

## Content generation pipeline

Generate several outputs from one notebook:

```bash
NB=abc123

# Structured outputs
nlm generate-guide "$NB" > guide.md
nlm study-guide "$NB" SOURCE_ID > study-guide.md
nlm faq "$NB" SOURCE_ID > faq.md
nlm timeline "$NB" SOURCE_ID > timeline.md
nlm briefing-doc "$NB" SOURCE_ID > briefing.md

# Artifact-backed outputs
nlm create-slides "$NB" "Presentation summary"
nlm report-suggestions "$NB"
nlm create-report "$NB" REPORT_TYPE "Executive brief"
nlm artifact list "$NB"

# Audio overview
nlm create-audio "$NB" "Professional summary"
nlm --direct-rpc audio download "$NB" overview.mp3
```

## Interactive chat with history

```bash
# Start a new chat session
nlm chat NOTEBOOK_ID

# Resume a previous session
nlm chat list NOTEBOOK_ID
nlm chat NOTEBOOK_ID CONVERSATION_ID

# Show server-side conversation history
nlm chat history NOTEBOOK_ID CONVERSATION_ID

# One-shot question
nlm chat NOTEBOOK_ID "What are the three main conclusions?"
```

## Structured chat for scripting

`--citations=json` turns the chat stream into typed JSON-lines events. Add
`--thinking` when you also want reasoning events.

```bash
# Capture just the answer text
nlm chat NB --citations=json "..." \
    | jq -rj 'select(.phase=="answer") | .text'

# Pull out citations with confidence scores
nlm chat NB --citations=json "..." \
    | jq -c 'select(.phase=="citation") | {i:.index, src:.source_id, conf:.confidence}'

# Route reasoning traces to a separate log file
nlm chat NB --citations=json --thinking "..." \
    | tee >(jq -r 'select(.phase=="thinking") | .text' > thinking.log) \
    | jq -rj 'select(.phase=="answer") | .text'
```

## Filter chat by source subset

```bash
# Ask one question against just the Q3-tagged sources
nlm source list NB | grep 'Q3' | nlm chat NB --source-ids - "What risks?"

# Discover sources, import them, then ask a focused question
nlm research NB "find Q3 risk material" \
    | jq -r 'select(.type=="source_discovered") | .url' \
    | nlm source add NB -

nlm source list NB | grep 'Q3' \
    | nlm chat NB --source-ids - "Summarize the risks."
```

## Branching on exit codes

```bash
nlm source add "$NB" "$file"
case $? in
  0) echo "added" ;;
  5) echo "source-cap reached; not retrying"; exit 1 ;;
  6) echo "transient error; sleeping"; sleep 30 ;;
  7) echo "still generating; polling"; sleep 10 ;;
  *) echo "unexpected error"; exit 1 ;;
esac
```

## Research workflow

```bash
# Create a research notebook
NB=$(nlm notebook create "Literature Review")

# Add papers
nlm source add "$NB" paper1.pdf
nlm source add "$NB" paper2.pdf
nlm source add "$NB" "https://arxiv.org/abs/2005.14165"

# Get source IDs
SOURCES=$(nlm source list "$NB" | awk '{print $1}' | tr '\n' ' ')

# Generate analysis
nlm summarize "$NB" $SOURCES
nlm critique "$NB" $SOURCES
nlm mindmap "$NB" $SOURCES

# Deep research
nlm research "$NB" "What are the methodological gaps across these papers?"

# Audio for review
nlm create-audio "$NB" "Academic tone, compare methodologies"
```

## Scripting with confirmation bypass

Use `-y` to skip confirmation prompts in scripts:

```bash
nlm notebook list | grep "2024-Q1" | awk '{print $1}' | while read -r id; do
    nlm -y notebook delete "$id"
done
```

## MCP integration

Use `nlm` as an MCP server so AI assistants can interact with NotebookLM:

```bash
# Test the MCP server manually
echo '{"jsonrpc":"2.0","id":1,"method":"tools/list"}' | nlm mcp

# Configure in Claude Desktop (see mcp.md for full setup)
```

## Debugging

```bash
# Enable debug output
nlm --debug notebook list
nlm --debug source list NOTEBOOK_ID

# Inspect raw protocol details
nlm --debug-dump-payload source list NOTEBOOK_ID
nlm --debug-parsing chat NOTEBOOK_ID "test"
nlm --debug-field-mapping source list NOTEBOOK_ID
```
