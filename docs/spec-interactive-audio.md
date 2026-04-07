# Spec: `nlm audio-interactive <notebook-id>`

Real-time bidirectional voice session with NotebookLM AI hosts over WebRTC.

## Command Interface

```
nlm audio-interactive <notebook-id> [flags]

Flags:
  --transcript-only   Skip audio playback, print transcript only
  --no-mic            Listen-only mode (no microphone input)
  --speaker <device>  Audio output device (default: system default)
  --mic <device>      Audio input device (default: system default)
  --timeout <dur>     Session timeout (default: 30m)
```

Output: live transcript to stdout, status/diagnostics to stderr. Audio
plays through the system audio device. The session runs until the user
sends SIGINT (Ctrl-C) or the timeout expires.

## Dependencies

| Package | Purpose | Version |
|---------|---------|---------|
| `github.com/pion/webrtc/v4` | WebRTC peer connection, ICE, DTLS, SCTP | v4.x |
| `github.com/hraban/opus` | Opus decode/encode (CGo, links libopus) | latest |
| `github.com/gordonklaus/portaudio` | Cross-platform audio I/O (CGo, links libportaudio) | latest |

CGo requirement: `libopus-dev` and `portaudio19-dev` (or Homebrew equivalents)
must be installed. Consider a build tag `interactive_audio` so the default build
does not require CGo:

```go
//go:build interactive_audio
```

## Architecture

```
cmd/nlm/main.go
  └─ case "audio-interactive": runInteractiveAudio(client, notebookID)

internal/interactiveaudio/
  session.go      — Session lifecycle: auth → SDP → run → teardown
  signaling.go    — FetchInteractivityToken + SDP exchange via batchexecute
  datachannel.go  — DataChannel protobuf decode/dispatch
  audio.go        — Opus decode → PortAudio playback + mic capture → Opus encode
  transcript.go   — Terminal transcript renderer (speaker attribution, timing)
  proto.go        — AgentCommsUserMessage protobuf definitions
```

## Phase 1: Session Authorization

### RPC: `Of0kDd` (FetchInteractivityToken)

```
POST https://notebooklm.google.com/_/LabsTailwindUi/data/batchexecute
rpcids=Of0kDd
payload: f.req=[[["Of0kDd","[]",null,"generic"]]]
```

Response (double-encoded JSON string):

```json
{
  "lifetimeDuration": "86400s",
  "iceServers": [
    {
      "urls": ["stun:74.125.247.128:3478", "stun:[2001:4860:4864:4:8000::]:3478"]
    },
    {
      "urls": [
        "turn:74.125.247.128:3478?transport=udp",
        "turn:[2001:4860:4864:4:8000::]:3478?transport=udp",
        "turn:74.125.247.128:3478?transport=tcp",
        "turn:[2001:4860:4864:4:8000::]:3478?transport=tcp"
      ],
      "username": "<ephemeral-credential>",
      "credential": "<ephemeral-password>"
    }
  ],
  "blockStatus": "NOT_BLOCKED",
  "iceTransportPolicy": "all"
}
```

Implementation:

```go
// signaling.go
type InteractivityToken struct {
    LifetimeDuration string      `json:"lifetimeDuration"`
    ICEServers       []ICEServer `json:"iceServers"`
    BlockStatus      string      `json:"blockStatus"`
    ICETransportPolicy string    `json:"iceTransportPolicy"`
}

type ICEServer struct {
    URLs       []string `json:"urls"`
    Username   string   `json:"username,omitempty"`
    Credential string   `json:"credential,omitempty"`
}

func (s *Session) FetchInteractivityToken(ctx context.Context) (*InteractivityToken, error) {
    // Execute Of0kDd via batchexecute with empty args "[]"
    // Parse double-encoded JSON response
    // Return structured token with ICE server configs
}
```

Add to `internal/notebooklm/rpc/rpc.go`:

```go
RPCFetchInteractivityToken = "Of0kDd" // FetchInteractivityToken - voice session auth + ICE config
RPCSDPExchange             = "eyWvXc" // SDP offer/answer exchange for WebRTC negotiation
```

## Phase 2: SDP Handshake

### Constraints (from capture analysis)

- **No ICE trickle.** All candidates must be bundled into the initial SDP offer.
- **1-second ICE gathering timeout.** The client gathers host, srflx, and relay
  candidates within this window, then sends whatever it has.
- **Bundled media.** Audio (Opus) and DataChannel (SCTP/DTLS) share one
  transport via BUNDLE.

### ICE Gathering

```go
// signaling.go
func (s *Session) gatherAndOffer(ctx context.Context, token *InteractivityToken) (string, error) {
    config := webrtc.Configuration{
        ICEServers: toWebRTCICEServers(token.ICEServers),
        ICETransportPolicy: webrtc.ICETransportPolicyAll,
    }
    pc, err := webrtc.NewPeerConnection(config)
    // ...

    // Add audio transceiver (sendrecv for bidirectional audio)
    _, err = pc.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio,
        webrtc.RTPTransceiverInit{Direction: webrtc.RTPTransceiverDirectionSendrecv})

    // Create DataChannel before offer (so it appears in SDP)
    dc, err := pc.CreateDataChannel("webrtc-datachannel", nil)

    // Create offer
    offer, err := pc.CreateOffer(nil)
    pc.SetLocalDescription(offer)

    // Wait for ICE gathering to complete (1s timeout)
    gatherCtx, cancel := context.WithTimeout(ctx, 1*time.Second)
    defer cancel()
    <-waitForGatheringComplete(pc, gatherCtx)

    // Return the offer with all bundled candidates
    return pc.LocalDescription().SDP, nil
}
```

### SDP Exchange: `eyWvXc`

```
POST https://notebooklm.google.com/_/LabsTailwindUi/data/batchexecute
rpcids=eyWvXc
payload: f.req=[[["eyWvXc","[{\"sdp\":\"<bundled-offer>\"}]",null,"generic"]]]
```

The server responds with a bundled SDP answer. From captures:
- Server selects Opus (payload type 111)
- Server ICE candidates: Google internal relay servers (e.g., `10.4.64.58`)
- BUNDLE confirmed for both audio and data

```go
func (s *Session) exchangeSDP(ctx context.Context, offerSDP string) (string, error) {
    // JSON-encode the SDP offer: [{"sdp":"v=0\r\n..."}]
    sdpJSON, _ := json.Marshal([]map[string]string{{"sdp": offerSDP}})

    // Execute eyWvXc via batchexecute
    resp, err := s.rpc.Do(ctx, rpc.Call{
        ID:   rpc.RPCSDPExchange,
        Args: string(sdpJSON),
    })

    // Parse response — extract SDP answer string
    // Apply as remote description: pc.SetRemoteDescription(answer)
    return answerSDP, nil
}
```

### Observed SDP Details (from captures)

Client offer includes:
- Audio: Opus/48000/2 (PT 111), G722, PCMU, PCMA, CN, telephone-event
- ICE candidates: host (mDNS-masked), srflx (STUN), relay (TURN)
- DTLS fingerprint: SHA-256
- SCTP: DataChannel on port 5000

Server answer selects:
- Audio: Opus only
- ICE: Google relay candidates at `10.4.64.{57,58}`
- BUNDLE: `BUNDLE 0 1` (audio + data on same transport)

## Phase 3: Audio Transport

### Receiving (Agent → User)

```go
// audio.go
func (s *Session) handleIncomingAudio(track *webrtc.TrackRemote) {
    decoder, _ := opus.NewDecoder(48000, 2)
    pcmBuf := make([]int16, 960*2) // 20ms at 48kHz stereo

    for {
        pkt, _, err := track.ReadRTP()
        if err != nil { break }

        n, _ := decoder.Decode(pkt.Payload, pcmBuf)
        s.audioOut.Write(pcmBuf[:n*2]) // Write to PortAudio stream
    }
}
```

### Sending (User → Agent)

```go
func (s *Session) captureAndSendAudio(track *webrtc.TrackLocalStaticRTP) {
    encoder, _ := opus.NewEncoder(48000, 1, opus.AppVoIP)
    pcmBuf := make([]int16, 960) // 20ms at 48kHz mono

    for {
        s.audioIn.Read(pcmBuf) // Read from PortAudio mic stream

        encoded := make([]byte, 1024)
        n, _ := encoder.Encode(pcmBuf, encoded)

        track.WriteRTP(&rtp.Packet{
            Header:  rtp.Header{PayloadType: 111, ...},
            Payload: encoded[:n],
        })
    }
}
```

### `--transcript-only` Mode

Skip PortAudio entirely. No CGo audio deps needed. The DataChannel still
delivers full transcripts. This mode works without libopus/libportaudio:

```go
if transcriptOnly {
    // Don't open audio devices
    // Mute outgoing audio track (send silence or don't add send transceiver)
    // DataChannel still delivers AGENT_UTTERANCE transcripts
}
```

## Phase 4: DataChannel Protocol

### Schema Provenance

The protobuf schema below is synthesized from two sources:

1. **Official**: `orchestration.proto` defines the base `AgentCommsUserMessage`
   with `AgentCommsMessageType type = 1` and `string content = 2`.
2. **Forensic**: Live WebRTC captures reveal that the server populates an
   undocumented wire field `4` with nested sub-messages, bypassing the generic
   `content` field entirely. The client must use the `type` enum at field 1 to
   determine which sub-struct to unmarshal from field 4.

### Canonical Protobuf Schema

```protobuf
syntax = "proto3";

package notebooklm.v1alpha1;

// DataChannelMessage is the outer envelope for all WebRTC DataChannel frames.
// Wire field 2 contains a sequenced inner message.
message DataChannelMessage {
    message Inner {
        int32 sequence_number = 1;   // monotonically increasing
        int32 payload_size = 2;      // byte size of the AgentCommsUserMessage
        AgentCommsUserMessage msg = 4;
    }
    Inner inner = 2;
}

// AgentCommsUserMessage is the protobuf transmitted over the WebRTC DataChannel
// for real-time bidirectional UI and transcript synchronization.
//
// The official orchestration.proto defines fields 1 (type) and 2 (content).
// The undocumented telemetry payload at field 4 is reconstructed from WebRTC
// network forensics — the server populates it dynamically based on the type.
message AgentCommsUserMessage {
    AgentCommsMessageType type = 1;
    string content = 2;

    // Undocumented telemetry payload. Dynamically populated based on
    // AgentCommsMessageType. The sub-field numbers match the type enum values.
    AgentCommsPayload payload = 4;
}

enum AgentCommsMessageType {
    AGENT_COMMS_MESSAGE_TYPE_UNSPECIFIED = 0;
    USER_UTTERANCE = 1;     // Real-time text transcript of user's speech
    AGENT_UTTERANCE = 2;    // Real-time text transcript of AI host's speech
    TTS_EVENT = 3;          // Text-to-speech lifecycle hooks
    SEND_AUDIO_EVENT = 4;   // Audio payload transmission triggers
    // 5 is unobserved
    PLAYBACK_EVENT = 6;     // Client-side media playback state sync
    MICROPHONE_EVENT = 7;   // Microphone active/muted state sync
    STATUS_MESSAGE = 8;     // Connection diagnostics and state updates
}

// Reconstructed sub-message encapsulating the dynamic DataChannel event payloads.
// Sub-field numbers correspond to the AgentCommsMessageType enum values.
message AgentCommsPayload {
    UserUtterance user_utterance = 1;
    AgentUtterance agent_utterance = 2;
    TTSEvent tts_event = 3;
    SendAudioEvent send_audio_event = 4;
    PlaybackEvent playback_event = 6;
    MicrophoneEvent microphone_event = 7;
    // StatusMessage at field 8 — structure TBD (12-23 byte payloads observed)
}

// Nested at field 4.1: Progressive speech-to-text of user's spoken audio.
// Each message replaces the previous (NOT cumulative).
message UserUtterance {
    string transcript = 3;
    bool is_final = 4;       // true = final transcript for this utterance
}

// Nested at field 4.2: AI host transcript chunks and speaker attributions.
// Segments are ~500-1000 bytes of spoken text.
message AgentUtterance {
    int32 speaker_id = 1;           // 2 = AI host (both speakers share this ID)
    repeated string speakers = 2;   // ["Host Speaker", "Expert Speaker"]
    string transcript_text = 3;     // Spoken text for this segment
    bool is_final = 4;              // true = last segment of this utterance
    string utterance_id = 5;        // e.g., "5932763328138406579"
}

// Nested at field 4.3: Coordinates frontend text highlighting with the Opus
// UDP stream. Provides precise epoch timestamps for sync.
message TTSEvent {
    int32 event_type = 1;    // 1 = start, 2 = end
    string utterance_id = 2; // matches AgentUtterance.utterance_id
    int32 segment_idx = 3;   // which segment (for multi-part utterances)
    TTSTimestamp timestamp = 4;
}

message TTSTimestamp {
    int64 epoch_sec = 1;
    int32 nanos = 2;
}

// Nested at field 4.4: Audio payload transmission triggers.
message SendAudioEvent {
    int32 trigger_type = 1;  // 1 = play, 2 = stop
    string utterance_id = 2; // matches AgentUtterance.utterance_id
}

// Nested at field 4.6: Client-side media playback states (open/close/mute).
message PlaybackEvent {
    string state1 = 1;  // playback init
    string state2 = 2;  // playback ended
    string state3 = 3;  // state update
}

// Nested at field 4.7: Synchronizes local microphone active/muted state.
message MicrophoneEvent {
    int32 status_code = 1;  // 3 = muted, others TBD (activate/deactivate)
}
```

### Observed UserUtterance Sequence

Progressive replacement (each message overwrites the previous):
```
"Hey."
"Hey there."
"Hey, there. This is."
"Hey there. This is a test message."
"Hey there. This is a test message. One two three."  (is_final=true)
```

### StatusMessage (field 4.8)

Small diagnostic messages (12-23 bytes). Appear between user speech end
and agent response start. Wire structure not yet fully mapped — need more
captures with varied session states.

### Schema vs Wire: Dispatch Architecture

The official `orchestration.proto` defines only `type` (field 1) and `content`
(field 2) on `AgentCommsUserMessage`. In practice, the server sends
base64-encoded `application/x-protobuf` payloads over the DataChannel that
populate the undocumented field 4 instead of `content`. The Go client must:

1. Unmarshal the outer `DataChannelMessage` envelope (field 2).
2. Read `AgentCommsMessageType` from field 1 of the inner message.
3. Based on the type, unmarshal the corresponding sub-message from field 4
   (e.g., type=2 → `AgentUtterance` at `payload.agent_utterance`).
4. Dispatch to the appropriate handler (transcript renderer, audio controller,
   or status logger).

The sub-field numbers within `AgentCommsPayload` intentionally mirror the
`AgentCommsMessageType` enum values (1→UserUtterance, 2→AgentUtterance, etc.),
which simplifies dispatch logic.

### Turn-Taking Protocol (from 50-message interactive capture)

```
 PLAYBACK_EVENT       → session init
 SEND_AUDIO_EVENT(s)  → overview audio playing
 PLAYBACK_EVENT       → state change
 AGENT_UTTERANCE      → "I think our listener's got something to say!"
 TTS_EVENT(s)         → TTS start/end
 SEND_AUDIO_EVENT(s)  → audio triggers
 MICROPHONE_EVENT     → mic activated (user about to speak)
 USER_UTTERANCE(s)    → progressive STT: "Hey." → "Hey there." → ... → final
 MICROPHONE_EVENT     → mic deactivated
 STATUS_MESSAGE(s)    → processing indicators
 AGENT_UTTERANCE      → agent response text
 TTS_EVENT            → TTS start
 SEND_AUDIO_EVENT(s)  → agent audio playing
 PLAYBACK_EVENT       → session end
```

## Phase 5: Terminal UI

### Transcript Rendering

```
$ nlm audio-interactive 2ed71b32-...
Connecting to interactive audio session...
  [ice] gathering candidates (1s timeout)
  [sdp] offer sent, answer received
  [connected]

🎙 Listening...

Host: Have you ever tried to test an application that talks to a remote
      API, and, uh, only to have the tests just completely fail because
      the network blipped for like half a second.

Expert: Oh, yeah. Constantly.

  [user speaking]
You: Hey there. This is a test message. One two three.

Host: Well, hello there! Welcome, welcome! We love it when you chime in.

^C
Session ended. Duration: 2m34s
```

### Display Rules

| Event | Rendering |
|-------|-----------|
| AGENT_UTTERANCE | `Speaker: <text>` — bold speaker name, wrap at terminal width |
| USER_UTTERANCE (progressive) | `\r  You: <text>` — overwrite in-place until is_final |
| USER_UTTERANCE (final) | `You: <text>\n` — commit line |
| MICROPHONE on | `\n  [user speaking]\n` in grey |
| MICROPHONE off | (clear status line) |
| STATUS_MESSAGE | stderr only, with `--debug` |
| TTS_EVENT | stderr only, with `--debug` |
| SEND_AUDIO_EVENT | stderr only, with `--debug` |
| PLAYBACK_EVENT | stderr only, with `--debug` |
| Connection state | stderr: `[ice]`, `[sdp]`, `[connected]`, `[disconnected]` |

Use the same ANSI grey (`\033[90m`) for status lines as the chat thinking
traces. Speaker names in bold (`\033[1m`).

### Transcript File Output

When stdout is not a TTY (piped), emit clean transcript without ANSI codes
and without progressive overwriting:

```
[HOST] Have you ever tried to test an application...
[EXPERT] Oh, yeah. Constantly.
[YOU] Hey there. This is a test message. One two three.
[HOST] Well, hello there! Welcome, welcome!
```

## Implementation Plan

### Step 1: Signaling (no CGo, no audio)

Add `internal/interactiveaudio/signaling.go`:
- `FetchInteractivityToken` via batchexecute `Of0kDd`
- `ExchangeSDP` via batchexecute `eyWvXc`
- Add RPC constants to `internal/notebooklm/rpc/rpc.go`

Testable with `--transcript-only` flag immediately after DataChannel works.

### Step 2: DataChannel + Transcript

Add `internal/interactiveaudio/datachannel.go` and `transcript.go`:
- Protobuf decode of `AgentCommsUserMessage`
- Dispatch to transcript renderer
- Terminal UI with progressive overwriting

This is the first user-visible milestone. With `--transcript-only`, the user
sees live transcripts without needing audio hardware.

### Step 3: Audio Playback (CGo)

Add `internal/interactiveaudio/audio.go` behind `//go:build interactive_audio`:
- Opus decode of incoming RTP packets
- PortAudio output stream
- Volume control / mute

### Step 4: Microphone Capture (CGo)

Extend `audio.go`:
- PortAudio input stream
- Opus encode
- RTP packetization and send
- Voice activity detection (optional — server may handle this)

### Step 5: Session Management

- Graceful shutdown on SIGINT (send DataChannel close, ICE disconnect)
- Reconnect on transient ICE failure
- Session timeout handling
- `--no-mic` mode (receive-only)

## Signaler Long-Poll (Optional)

The web UI maintains a persistent connection to
`signaler-pa.clients6.google.com` using Google's Channel API (CVER=22, VER=8)
for real-time state sync during the interactive session.

From captures, the `chooseServer` call passes `["tailwind"]` with the notebook
ID and receives a session token. The `multi-watch/channel` endpoint then
long-polls for updates.

This may be required for session keepalive or state sync. If the WebRTC
DataChannel alone is insufficient for maintaining the session, implement this
as a background HTTP long-poll goroutine.

## Open Questions

1. **Microphone activation protocol.** Does the client send a MICROPHONE_EVENT
   to signal "user wants to speak", or does the server detect voice activity
   from the audio stream and send MICROPHONE_EVENT as acknowledgment?

2. **DataChannel send.** The captures only show RECEIVE messages (the JS
   monkey-patch captured incoming messages). What protobuf messages does the
   client send on the DataChannel? At minimum: PLAYBACK_EVENT, MICROPHONE_EVENT.
   May also need to send USER_UTTERANCE or audio control messages.

3. **Signaler necessity.** Is the `signaler-pa.clients6.google.com` long-poll
   required for session maintenance, or is the WebRTC connection self-sustaining
   once established?

4. **Audio codec params.** The SDP negotiates Opus at 48000/2 (stereo). Is the
   incoming audio actually stereo, or mono packed as stereo? What frame size
   does the server expect for outgoing audio?

5. **Session resumption.** If the WebRTC connection drops, can we re-negotiate
   with a new SDP offer using the same interactivity token (valid 24h), or must
   we fetch a new token?

6. **`ub2Bae` relevance.** The undocumented RPC (412KB response, now identified
   as `ListCollections`) — does it play any role in interactive audio session
   setup?
