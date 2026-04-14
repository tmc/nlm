# NLM CLI — Remaining Tasks

All commands compile and have implementations. This document tracks what
needs fixing, testing, or finishing to make every command fully operational.

## Notebook Operations

- [x] `list` / `ls` — list notebooks
- [x] `create <title>` — create notebook
- [x] `rm <id>` — delete notebook
- [x] `analytics <id>` — project analytics
- [x] `list-featured` — list featured notebooks

## Source Operations

- [x] `sources <id>` — list sources
- [x] `add <id> <file|url|text>` — add source (file upload, URL, paste)
- [x] `rm-source <id> <source-id>` — remove source
- [x] `rename-source <source-id> <name>` — rename source
- [x] `refresh-source <id> <source-id>` — refresh source content
- [x] `check-source <source-id>` — check source freshness
- [x] `discover-sources <id> <query>` — discover related sources

## Note Operations

- [x] `notes <id>` — list notes
- [x] `new-note <id> <title> [content]` — create note (supports stdin)
- [x] `update-note <id> <note-id> <content> <title>` — update note
- [x] `rm-note <id> <note-id>` — remove note

## Audio Operations

- [x] `create-audio <id> <instructions>` — create audio overview
- [x] `audio-get <id>` — get audio overview status
- [x] `audio-rm <id>` — delete audio overview
- [x] `audio-list <id>` — list audio overviews
- [ ] `audio-download <id> [filename]` — download audio
  - [ ] `DownloadAudioOverview` response parsing returns 0 elements
  - [ ] Investigate wire format of audio data response (may need HAR capture)
  - [ ] Verify `SaveAudioToFile` handles the actual response structure
- [ ] `audio-share <id>` — share audio overview
  - [ ] `ShareAudio` returns empty share URL
  - [ ] Investigate `ShareOption` wire format — may need different share type enum
  - [ ] Compare with HAR capture of web UI share flow

## Video Operations

- [x] `create-video <id> <instructions>` — create video overview
- [x] `video-list <id>` — list video overviews
- [ ] `video-download <id> [filename]` — download video
  - [ ] Verify `DownloadVideoOverview` actually returns a URL or base64 data
  - [ ] Test `DownloadVideoWithAuth` cookie-based download path
  - [ ] Confirm `SaveVideoToFile` handles non-URL formats

## Artifact Operations

- [ ] `create-artifact <id> <type>` — create artifact
  - [ ] Verify proto enum values for artifact types (note, audio, report, app)
  - [ ] Server returns 400 for some types — determine which are valid
  - [ ] Test each type individually against live API
- [ ] `get-artifact <artifact-id>` — get artifact details
  - [ ] Returns API endpoint errors — investigate RPC wire format
  - [ ] May need direct RPC fallback like `list-artifacts`
- [x] `list-artifacts <id>` — list artifacts
- [x] `rename-artifact <artifact-id> <title>` — rename artifact
- [x] `delete-artifact <artifact-id>` — delete artifact

## Generation Operations

- [x] `generate-guide <id>` — generate notebook guide
- [x] `generate-outline <id>` — generate report suggestions
- [x] `generate-section <id>` — generate report section
- [x] `generate-magic <id> <source-ids...>` — generate magic view
- [x] `generate-mindmap <id> <source-ids...>` — generate mindmap
- [x] `generate-chat <id> <prompt>` — one-shot chat (non-interactive)
- [x] Content transformations (all use `actOnSources`):
  - [x] `rephrase`, `expand`, `summarize`, `critique`, `brainstorm`
  - [x] `verify`, `explain`, `outline`, `study-guide`, `faq`
  - [x] `briefing-doc`, `mindmap`, `timeline`, `toc`

## Chat Operations

- [x] `chat <id>` — interactive chat
- [x] `chat <id> <prompt>` — one-shot chat
- [x] `chat <id> <conv-id>` — resume conversation
- [x] `chat-list [id]` — list conversations (server or local)
- [x] `delete-chat <id>` — delete chat history
- [ ] `chat-config <id> <setting> [value]` — configure chat behavior
  - [ ] Verify `SetChatConfig` via `MutateProject` (s0tc2d) actually applies
  - [ ] Test `goal default` / `goal custom "prompt"` paths
  - [ ] Test `length default` / `length longer` / `length shorter` paths
  - [ ] Verify ChatGoal enum values match server expectations

### Chat Streaming Improvements

- [x] Real-time streaming (replaced `io.ReadAll` with `bufio.Scanner`)
- [x] Phase-aware parsing (thinking vs answer detection)
- [ ] Thinking trace display
  - [ ] Verify thinking chunks actually start with `**` in live stream
  - [ ] Test `--verbose` flag shows full thinking text in grey
  - [ ] Test default mode shows header-only with carriage-return overwrite
  - [ ] Test non-TTY mode silently drops thinking, streams answer only
- [x] First-answer-chunk truncation bug
  - [x] Parser now tracks thinking and answer phases separately
  - [x] Added unit coverage for reasoning-to-answer transition
- [ ] Citation extraction from stream
  - [ ] Parse `[[formatting/citation data]]` from inner JSON position [0][4]
  - [ ] Map citation references to source excerpts
  - [ ] Populate `ChatMessage.Citations` in local session storage
- [ ] Follow-up suggestion chips
  - [ ] Extract from stream or via `GenerateNotebookGuide` (VfAZjd)
  - [ ] Display as clickable options in interactive mode

### Interactive Chat Features

- [x] `/exit`, `/quit` — exit chat
- [x] `/clear` — clear conversation
- [x] `/history` — show message history
- [x] `/reset` — reset conversation state
- [x] `/new` — start new conversation
- [x] `/fork` — fork current conversation
- [x] `/conversations` — list conversations
- [x] `/save` — save session
- [x] `/help` — show help
- [x] `/multiline` — toggle multiline input
- [ ] Chat rating / feedback
  - [ ] Intentionally omitted from interactive chat for now
  - [ ] Keep standalone `feedback <message>` as the current feedback path

## Configuration Operations

- [x] `set-instructions <id> <prompt>` — set system instructions
- [x] `get-instructions <id>` — get current instructions

## Research Operations

- [x] `research <id> <query>` — deep research with polling
  - [ ] Verify output formatting is clean (may need polish)

## Sharing Operations

- [x] `share <id>` — share notebook (public link)
- [x] `share-private <id>` — share notebook (private link)
- [x] `share-details <share-id>` — get share details

## Other Operations

- [x] `feedback <message>` — submit feedback
- [x] `hb` — heartbeat
- [x] `auth [profile]` — authentication
- [x] `refresh` — refresh credentials

## Cross-Cutting Concerns

- [ ] Proto/codegen alignment
  - [ ] `buf generate` clobbers hand-written encoders in `gen/method/` and `gen/service/`
  - [ ] Need backup/restore workflow or move encoders out of `gen/`
- [ ] Test coverage
  - [ ] 8/10 scripttest files pass; `input_handling.txt` and `network_failures.txt` fail
  - [ ] Add integration tests for chat streaming
  - [ ] Add tests for audio-download, audio-share, artifact operations
- [ ] Error messages
  - [ ] Auth expiry mid-session gives unclear errors
  - [ ] Consider auto-refresh on 401/Unauthenticated responses

## Interactive Audio

- [x] `audio-interactive <id>` command surface in help, validation, and dispatch
- [x] `--transcript-only`, `--no-mic`, `--speaker`, `--mic`, `--timeout`, `--help` parsing and usage output
- [x] `FetchInteractivityToken` (Of0kDd) — voice session auth
- [x] `eyWvXc` RPC wiring for WebRTC SDP offer/answer negotiation
- [ ] Live NotebookLM acceptance of the `eyWvXc` SDP offer
- [x] Decode captured WebRTC DataChannel transcripts and render them in `--transcript-only` mode
- [x] macOS backend scaffold uses `github.com/tmc/apple/avfaudio` for future playback/capture wiring
- [ ] Remote audio playback (Opus decode + local speaker output)
- [ ] Microphone capture / outbound audio encode
- [ ] Live end-to-end verification against NotebookLM
- [x] See `docs/spec-interactive-audio.md` for protocol details
