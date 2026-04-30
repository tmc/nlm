---
title: Interactive Audio Wire Format
date: 2026-04-29
---

# Interactive Audio Wire Format

`nlm audio-interactive <notebook-id>` opens a real-time bidirectional voice
session with the NotebookLM AI hosts over WebRTC.

This document describes the **wire-format protocol** used to set up and run
that session: the batchexecute signaling RPCs, the SDP constraints, and the
DataChannel protobuf schema. The implementation lives in
`internal/interactiveaudio/` and is the source of truth for behavior; this
file is the source of truth for the on-the-wire shapes.

For the user-facing CLI surface, run `nlm audio-interactive --help`.

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

The token is valid for 24 hours.

RPC constants live in `internal/notebooklm/rpc/rpc.go`:

```go
RPCFetchInteractivityToken = "Of0kDd"
RPCSDPExchange             = "eyWvXc"
```

## Phase 2: SDP Handshake

### Constraints (from capture analysis)

- **No ICE trickle.** All candidates must be bundled into the initial SDP offer.
- **1-second ICE gathering timeout.** The client gathers host, srflx, and relay
  candidates within this window, then sends whatever it has.
- **Bundled media.** Audio (Opus) and DataChannel (SCTP/DTLS) share one
  transport via BUNDLE.

### SDP Exchange: `eyWvXc`

```
POST https://notebooklm.google.com/_/LabsTailwindUi/data/batchexecute
rpcids=eyWvXc
payload: f.req=[[["eyWvXc","[{\"sdp\":\"<bundled-offer>\"}]",null,"generic"]]]
```

The server responds with a bundled SDP answer. From captures:

- Server selects Opus (payload type 111).
- Server ICE candidates are Google internal relay servers (e.g., `10.4.64.58`).
- BUNDLE is confirmed for both audio and data.

### Observed SDP Details

Client offer includes:

- Audio: Opus/48000/2 (PT 111), G722, PCMU, PCMA, CN, telephone-event
- ICE candidates: host (mDNS-masked), srflx (STUN), relay (TURN)
- DTLS fingerprint: SHA-256
- SCTP: DataChannel on port 5000

Server answer selects:

- Audio: Opus only
- ICE: Google relay candidates at `10.4.64.{57,58}`
- BUNDLE: `BUNDLE 0 1` (audio + data on same transport)

## Phase 3: DataChannel Protocol

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
populate the undocumented field 4 instead of `content`. The client must:

1. Unmarshal the outer `DataChannelMessage` envelope (field 2).
2. Read `AgentCommsMessageType` from field 1 of the inner message.
3. Based on the type, unmarshal the corresponding sub-message from field 4
   (e.g., type=2 → `AgentUtterance` at `payload.agent_utterance`).
4. Dispatch to the appropriate handler (transcript renderer, audio controller,
   or status logger).

The sub-field numbers within `AgentCommsPayload` intentionally mirror the
`AgentCommsMessageType` enum values (1→UserUtterance, 2→AgentUtterance, etc.),
which simplifies dispatch logic.

### Turn-Taking Protocol

Observed message order across a 50-message interactive capture:

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

## Signaler Long-Poll (optional)

The web UI maintains a persistent connection to
`signaler-pa.clients6.google.com` using Google's Channel API (CVER=22, VER=8)
for real-time state sync during the interactive session.

From captures, the `chooseServer` call passes `["tailwind"]` with the notebook
ID and receives a session token. The `multi-watch/channel` endpoint then
long-polls for updates.

The current `internal/interactiveaudio/` implementation does not rely on this
channel — the WebRTC DataChannel alone has been sufficient for live sessions.
This section is retained as forensic context if a future change requires
session-keepalive or out-of-band state sync.

## Open Wire-Format Questions

These are the items that future captures could resolve. They do not block any
current functionality.

1. **StatusMessage (field 4.8) structure.** 12-23 byte payloads observed
   between user speech end and agent response start. Schema unmapped.
2. **MicrophoneEvent status_code values.** `3 = muted` is observed; the
   activate / deactivate codes are not yet identified.
3. **Client-to-server DataChannel messages.** The captures only recorded the
   server-to-client direction (the JS monkey-patch hooked incoming messages).
   The exact protobuf shape the client sends back — beyond the obvious
   `PLAYBACK_EVENT` / `MICROPHONE_EVENT` — is not yet documented here.
4. **Session resumption.** It is not yet known whether a dropped WebRTC
   connection can be re-negotiated with a new SDP offer using the same
   24-hour interactivity token, or whether a new token must be fetched.
