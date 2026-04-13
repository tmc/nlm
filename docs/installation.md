---
title: Installation
---
# Installation

## Requirements

- Go 1.25 or later
- A Google account with access to [NotebookLM](https://notebooklm.google.com)
- Chrome or Brave browser (for `nlm auth`)

## Install from source

```bash
go install github.com/tmc/nlm/cmd/nlm@latest
```

Or clone and build:

```bash
git clone https://github.com/tmc/nlm.git
cd nlm
go build -o nlm ./cmd/nlm
```

## Verify

```bash
nlm --help
```

## Shell completion

nlm uses standard flag parsing. Tab completion is not built in, but you can create a wrapper:

```bash
# Bash alias with common commands
alias nlm-ls='nlm ls'
alias nlm-chat='nlm chat'
```
