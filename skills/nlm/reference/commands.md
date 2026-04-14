# nlm Command Reference

## Notebooks
```
nlm list                           # List notebooks
nlm create <title>                 # Create notebook
nlm rm -y <id>                     # Delete notebook
nlm analytics <id>                 # Show notebook analytics
```

## Sources
```
nlm sources <id>                   # List sources in notebook
nlm add <id> <file|url|->         # Add source (use --name to set title)
nlm rm-source -y <id> <src-id>    # Remove source
nlm rename-source <src-id> <name>  # Rename source
nlm refresh-source <id> <src-id>   # Refresh source content
nlm check-source <src-id>          # Check source freshness
nlm discover-sources <id> <query>  # Discover relevant sources
```

## Notes
```
nlm notes <id>                     # List notes
nlm read-note <id> <note-id>      # Read full note content
nlm new-note <id> <title>          # Create new note
nlm update-note <id> <note-id> <content> <title>  # Edit note
nlm rm-note <id> <note-id>         # Remove note
```

## Content Creation
```
nlm create-audio <id> <instr>      # Create audio overview
nlm create-video <id> <instr>      # Create video overview
nlm create-slides <id> <instr>     # Create slide deck
```

## Audio
```
nlm audio-list <id>                # List audio overviews with status
nlm audio-get <id>                 # Get audio overview details
nlm audio-download <id> [file]     # Download audio (needs --direct-rpc)
nlm audio-rm <id>                  # Delete audio overview
nlm audio-share <id>               # Share audio overview
```

## Video
```
nlm video-list <id>                # List video overviews with status
nlm video-download <id> [file]     # Download video (needs --direct-rpc)
```

## Artifacts
```
nlm artifacts <id>                 # List artifacts in notebook
nlm get-artifact <artifact-id>     # Get artifact details
nlm rename-artifact <id> <title>   # Rename artifact
nlm delete-artifact <id>           # Delete artifact
```

## Guidebooks
```
nlm guidebooks                     # List all guidebooks
nlm guidebook <id>                 # Get guidebook content
nlm guidebook-publish <id>         # Publish a guidebook
nlm guidebook-share <id>           # Share a guidebook
nlm guidebook-ask <id> <question>  # Ask a guidebook
nlm guidebook-rm <id>              # Delete a guidebook
```

## Chat & Generation
```
nlm chat <id>                      # Interactive chat session
nlm generate-chat <id> <prompt>    # One-shot chat generation
nlm chat-list [id]                 # List chat sessions
nlm delete-chat <id>               # Delete server-side chat history
nlm chat-config <id> <setting> [value]  # Configure chat settings
nlm set-instructions <id> <text>   # Set notebook chat instructions
nlm get-instructions <id>          # Show current instructions
nlm generate-guide <id>            # Comprehensive guide
nlm generate-outline <id>          # Content outline
nlm generate-section <id>          # New section
nlm generate-magic <id> <src-ids>  # Magic view from sources
```

## Content Transformation

All take `<notebook-id> <source-ids...>`:

```
nlm summarize     # Summarize sources
nlm explain       # Explain concepts
nlm rephrase      # Rephrase content
nlm expand        # Expand on content
nlm critique      # Critique content
nlm brainstorm    # Brainstorm ideas
nlm verify        # Verify facts
nlm outline       # Create outline
nlm study-guide   # Study guide
nlm faq           # Generate FAQ
nlm briefing-doc  # Briefing document
nlm mindmap       # Interactive mindmap
nlm timeline      # Timeline from sources
nlm toc           # Table of contents
```

## Sharing
```
nlm share <id>                     # Share notebook publicly
nlm share-private <id>             # Share privately
nlm share-details <share-id>       # Get share details
```

## Research
```
nlm research <id> <query>          # Deep research and poll for results
```

## System
```
nlm auth                           # Setup authentication (opens browser)
nlm auth --authuser 1              # Auth with secondary Google account
nlm refresh                        # Refresh auth credentials
nlm mcp                            # Start MCP server (stdin/stdout)
nlm hb                             # Heartbeat check
nlm feedback <msg>                 # Submit feedback
```

## Flags

| Flag | Description |
|------|-------------|
| `-y` | Skip confirmation prompts |
| `--debug` | Show detailed request/response info |
| `--direct-rpc` | Required for audio/video download |
| `--authuser N` | Multi-account Google profiles |
| `--name NAME` | Custom name for added sources |
| `--mime TYPE` | MIME type for stdin content |
| `--thinking` | Show reasoning headers in chat |
| `--verbose` | Show full reasoning traces |
| `--history` | Show previous chat on start |
