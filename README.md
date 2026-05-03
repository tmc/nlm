# nlm

Command-line interface and MCP server for [Google NotebookLM](https://notebooklm.google.com).

## Install

```
go install github.com/tmc/nlm/cmd/nlm@latest
```

## Authentication

nlm reads browser cookies from Chrome, Brave, or Edge. Run any command and it will
attempt to find valid credentials automatically. You can also pass credentials
explicitly:

```
nlm -cookies 'SID=...; HSID=...; SSID=...' list
nlm -auth <auth-token> list
```

For multi-account setups, use `--authuser N` (0-indexed) to select the Google account.

## Usage

```
nlm <command> [arguments]
```

### Notebooks

```
nlm list                              # list all notebooks
nlm create "My Notebook"              # create a notebook
nlm rm <notebook-id>                  # delete a notebook
nlm analytics <notebook-id>           # show notebook analytics
```

### Sources

```
nlm sources <notebook-id>                          # list sources
nlm add <notebook-id> https://example.com          # add URL source
nlm add <notebook-id> document.pdf                 # add file source
nlm add <notebook-id> --text "Title" "Content"     # add text source
nlm rm-source <notebook-id> <source-id>            # remove source
```

### Notes

```
nlm notes <notebook-id>                                    # list notes
nlm read-note <notebook-id> <note-id>                      # read note content
nlm new-note <notebook-id> "Title" "Content"               # create note
nlm update-note <notebook-id> <note-id> "Content" "Title"  # edit note
nlm rm-note <note-id>                                      # delete note
```

### Artifact Creation

```
nlm create-audio <notebook-id> "deep dive on topic X"    # audio overview
nlm create-video <notebook-id> "whiteboard walkthrough"   # video overview
nlm create-slides <notebook-id> "presentation summary"    # slide deck
nlm create-flashcards <notebook-id>                       # flashcards
nlm create-infographic <notebook-id> "visual summary"     # infographic
```

### Artifacts

```
nlm artifacts <notebook-id>                         # list artifacts
nlm get-artifact <artifact-id>                      # get artifact details
nlm download-artifact <notebook-id> <artifact-id>   # download artifact files
nlm download-infographic <notebook-id> <artifact-id>  # download infographic image
nlm download-slides <notebook-id> <artifact-id>      # download slide deck files
nlm download-flashcards <notebook-id> <artifact-id>  # download flashcard media
nlm rename-artifact <artifact-id> "New Title"       # rename artifact
nlm delete-artifact <artifact-id>                   # delete artifact
```

### Audio and Video

```
nlm audio-list <notebook-id>           # list audio overviews
nlm audio-get <notebook-id>            # get audio details
nlm audio-share <notebook-id>          # share audio
nlm video-list <notebook-id>           # list video overviews
```

### Chat

```
nlm chat <notebook-id>                          # interactive chat session
nlm chat <notebook-id> "What is this about?"    # one-shot question
nlm generate-chat <notebook-id> "summarize"     # free-form generation
```

### Content Transformation

```
nlm summarize <notebook-id>     nlm critique <notebook-id>
nlm rephrase <notebook-id>      nlm brainstorm <notebook-id>
nlm expand <notebook-id>        nlm verify <notebook-id>
nlm explain <notebook-id>       nlm outline <notebook-id>
nlm study-guide <notebook-id>   nlm faq <notebook-id>
nlm timeline <notebook-id>      nlm toc <notebook-id>
nlm briefing-doc <notebook-id>  nlm mindmap <notebook-id>
```

### Sharing

```
nlm share <notebook-id>              # share publicly
nlm share-private <notebook-id>      # share with restricted access
nlm share-details <share-id>         # view collaborators and permissions
```

### Deep Research

```
nlm research <notebook-id> "research query"   # start deep research session
```

## MCP Server

nlm includes a built-in [Model Context Protocol](https://modelcontextprotocol.io/) server
that exposes NotebookLM operations as tools for AI assistants.

```
nlm mcp
```

Configure it in your MCP client (e.g. Claude Code):

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

Available MCP tools: `list_notebooks`, `list_sources`, `list_notes`, `list_artifacts`,
`create_notebook`, `create_note`, `create_audio_overview`, `create_video_overview`,
`create_slide_deck`, `create_infographic`, `generate_chat`, `read_note`,
`set_instructions`, `get_instructions`, `start_deep_research`,
`poll_deep_research`, and more.

## Flags

```
-auth string       Auth token
-cookies string    Browser cookies (SID, HSID, SSID)
-authuser int      Google account index (default 0)
-debug             Enable debug output
-direct-rpc        Use direct RPC calls (required for some commands)
-y                 Skip confirmation prompts
```

## Package Structure

```
cmd/nlm/                    CLI entry point
internal/
  notebooklm/api/           High-level API client
  notebooklm/rpc/           Low-level RPC client
  batchexecute/             Google batchexecute protocol
  beprotojson/              Proto <-> batchexecute JSON marshaling
  nlmmcp/                   MCP server implementation
  auth/                     Browser cookie extraction
  interactiveaudio/         WebRTC interactive audio (experimental)
gen/
  method/                   RPC argument encoders
  service/                  Generated service clients
  notebooklm/v1alpha1/      Protocol buffer definitions
```

## License

MIT
