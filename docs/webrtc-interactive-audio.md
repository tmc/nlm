# Interactive Audio (WebRTC) Implementation Notes

Deferred for later implementation. Focus is on chat for now.

## Signaling and WebRTC Handshake

1. **Authorization:** Execute `FetchInteractivityToken` RPC (`Of0kDd`) with empty payload (`[]`) to retrieve session token.
2. **SDP Offer Generation:** Generate SDP offer with vanilla ICE gathering (1s timeout). Bundle all ICE candidates into the initial SDP offer.
3. **Negotiation:** Transmit bundled SDP offer via `eyWvXc` RPC. Server responds with bundled SDP answer containing ICE candidates and Opus audio configs.

## Media and DataChannel

- **MediaStream (UDP):** Opus-encoded audio frames.
- **DataChannel:** Real-time bidirectional UI and transcript synchronization using `AgentCommsUserMessage` protobuf.

## AgentCommsUserMessage Types

| Type | Name | Description |
|------|------|-------------|
| 1 | `USER_UTTERANCE` | Real-time text transcript of user's speech |
| 2 | `AGENT_UTTERANCE` | Real-time text transcript of AI host's speech |
| 3 | `TTS_EVENT` | Text-to-speech lifecycle hooks |
| 4 | `SEND_AUDIO_EVENT` | Audio payload transmission triggers |
| 6 | `PLAYBACK_EVENT` | Client-side media playback state sync |
| 7 | `MICROPHONE_EVENT` | Microphone active/muted state sync |
| 8 | `STATUS_MESSAGE` | Connection diagnostics and state updates |

## Key Constraints

- No secondary signaling channels or ICE trickling
- Bundled negotiation sequence only
- Standard text chat uses basic role integers (1=user, 2=assistant)
- Interactive audio requires the full `AgentCommsUserMessage` schema for DataChannel decoding
