# HAR Capture Analysis — 2026-04-07

Captures from `new-cdp-tests` context with enhanced cdp recorder (response
bodies, gRPC-Web streams, WebRTC SDP). Files copied to `docs/captures/`.

## Files Captured

| File | Entries | Size | Content |
|------|---------|------|---------|
| `notebooklm.google.com.jsonl` | 26 | 7.1MB | All batchexecute RPCs + gRPC-Web chat stream |
| `_capture.jsonl` | 3 | 8KB | WebRTC SDP offer/answer (custom capture format) |
| `webrtc-signaling.jsonl` | 3 | 9KB | SDP flow as synthetic HAR entries |
| `signaler-pa.clients6.google.com.jsonl` | 6 | 14KB | Long-poll signaling channel |

## Entry Index — notebooklm.google.com.jsonl

| # | RPC ID | Operation | POST | Response | Notes |
|---|--------|-----------|------|----------|-------|
| 0 | — | Page load | — | 304KB | Notebook HTML |
| 1 | — | Manifest | — | 460B | Static branding |
| 2 | `sqTeoe` | GetAudioFormats | 368B | 3KB | Audio format catalog |
| 3 | `rLM1Ne` | GetProject | 268B | 17KB | Full project data |
| 4 | `ZwVcOc` | GetOrCreateAccount | 280B | 248B | Account init |
| 5 | `hPTbtc` | GetConversations | 256B | 187B | Conversation list |
| 6 | `JFMDGd` | GetProjectDetails | 244B | 328B | Share details |
| 7 | `e3bVqc` | GetConversations (polymorphic) | 248B | 1.7MB | Full conversation data |
| 8 | `gArtLc` | ListArtifacts | 544B | 165KB | All artifacts |
| 9 | `ub2Bae` | **Unknown** | 176B | 412KB | Undocumented RPC |
| 10 | `wXbhsf` | ListRecentlyViewedProjects | 200B | 1.5MB | Project list |
| 11 | `ozz5Z` | LogEvent | 352B | 1KB | Telemetry |
| 12-13 | — | Static assets | — | 68KB | Icons, animations |
| 14 | `khqZz` | GetConversationHistory | 264B | 435KB | Message history |
| 15 | `VfAZjd` | GenerateNotebookGuide | 244B | 2KB | Guide + suggestions |
| 16 | `cFji9` | GetNotes | 260B | 312KB | All notes |
| 17-21 | — | Static assets | — | 99KB | Logos, GIFs, animations |
| 22 | **gRPC-Web** | GenerateFreeFormStreamed | 47KB | 1.3MB | **Full chat stream** |
| 23 | `Of0kDd` | FetchInteractivityToken | 164B | 760B | ICE/TURN servers |
| 24 | `eyWvXc` | SDP offer/answer exchange | 8.8KB | 3.2KB | WebRTC negotiation |
| 25 | — | Static asset | — | 12KB | User speaking animation |

## Key Discovery: `e3bVqc` is Polymorphic

RPC `e3bVqc` serves double duty:
- **As DeleteChatHistory**: Deletes all conversations (our existing use)
- **As GetConversations (full)**: Returns complete conversation data with message
  content (1.7MB in this capture)

The args likely differ — the GetConversations variant probably sends the project
ID with a different flag than the delete variant. Need to diff the POST bodies.

## New RPC: `ub2Bae`

Entry 9 returned 412KB. Not in our RPC mapping. Need to investigate — could be
a page-load RPC for fetching all project metadata, or a new endpoint.

---

## Chat Stream Structure (Entry 22)

### Wire Format
```
)]}'                          ← anti-XSSI prefix
995                           ← decimal byte count of next JSON line
[["wrb.fr",null,"<inner>"]]  ← envelope with double-encoded inner JSON
42                            ← next frame length
...
```

62 total chunks: 7 thinking + 38 answer + metadata frames.

### Phase Detection — Confirmed

Thinking chunks start with `**bold header**`:
```
**Assessing the Prompt's Core**
**Deconstructing the WebRTC Flow**
**Mapping DataChannel Message Types**
**Orchestrating the CLI's Logic**
**Structuring the Meta-Trace**
**Charting the Implementation Path**
**Clarifying the Data Flow**
```

First answer chunk does NOT start with `**`:
```
<meta_cognitive_trace>
```

Answer chunks are cumulative — each extends the previous text. Our delta-based
parser heuristic (check `strings.HasPrefix(text, lastText)`) is correct.

### Inner JSON Structure (Final Chunk)

```
[                                           ← outer array
  [                                         ← [0]: message data
    "cumulative answer text",               ← [0][0]: full answer (5627 chars)
    null,                                   ← [0][1]: unused
    ["conv_id", "msg_id", seq_num],         ← [0][2]: conversation metadata
    null,                                   ← [0][3]: unused
    [rich formatting/citation spans]        ← [0][4]: char-offset annotations
  ],
  [                                         ← [1]: citation details (array)
    [null, null, 0.994, [[null, start, end]], [[[start, end, [text...]]]]], ...
  ],                                        ← each: confidence score + source text excerpts
  [                                         ← [2]: citation-to-source mapping
    [[null, start, end], [source_indices]], ...
  ],                                        ← char ranges → which sources cited
  [                                         ← [3]: follow-up suggestion chips
    ["question 1", "question 2", "question 3"]
  ],
  true,                                     ← [4]: completion flag
  [                                         ← [5]: follow-up chips with type codes
    [["question 1", 9], ["question 2", 9], ["question 3", 9]]
  ]
]
```

### Citation Structure Detail

Position `[1]` contains per-citation entries:
```json
[
  null,
  null,
  0.9937714107754594,           // confidence score
  [[null, 12700, 13376]],       // character range in answer text
  [[[12700, 13376, [            // source text excerpts for this citation
    [[12700, 12738, ["Real-Time Interactive Audio via WebRTC", [true]]],
     [12738, 12790, ["additional excerpt text"]]]
  ]]]]
]
```

Position `[2]` maps character ranges to source indices:
```json
[[null, 2111, 2272], [0]]       // chars 2111-2272 cite source index 0
[[null, 2272, 2366], [1]]       // chars 2272-2366 cite source index 1
[[null, 2466, 2533], [0, 1, 2]] // chars 2466-2533 cite sources 0, 1, 2
```

### Follow-Up Suggestion Chips

Position `[3][0]` — plain strings:
```json
[
  "How do I encode source IDs in three-level nested arrays?",
  "What headers are required for gRPC-Web chat streaming?",
  "Tell me more about the AgentCommsUserMessage message types."
]
```

Position `[5][0]` — with type codes:
```json
[
  ["How do I encode source IDs in three-level nested arrays?", 9],
  ["What headers are required for gRPC-Web chat streaming?", 9],
  ["Tell me more about the AgentCommsUserMessage message types.", 9]
]
```

Type code `9` appears to be "question" type. Other codes TBD.

---

## Audio Format Catalog (Entry 2: `sqTeoe`)

GetAudioFormats returns three categories of content types:

### Audio Formats (position [0][0])
| ID | Name | Description |
|----|------|-------------|
| 1 | Deep Dive | Lively conversation between two hosts, unpacking topics |
| 2 | Brief | Bite-sized overview of core ideas |
| 3 | Critique | Expert review with constructive feedback |
| 4 | Debate | Thoughtful debate illuminating different perspectives |

### Video Formats (position [0][1])
| ID | Name | Description |
|----|------|-------------|
| 1 | Explainer | Structured, comprehensive overview |
| 2 | Brief | Bite-sized overview |
| 3 | Cinematic | Rich, immersive experience with visuals and storytelling |

### Slide Formats (position [0][2])
| ID | Name | Description |
|----|------|-------------|
| 1 | Detailed Deck | Comprehensive with full text, for reading/emailing |
| 2 | Presenter Slides | Clean visuals with talking points |

### Document Templates (position [0][3])
Array of named templates: Briefing Doc, FAQ, Study Guide, Timeline, etc.

---

## WebRTC Interactivity Token (Entry 23: `Of0kDd`)

Response is a JSON string containing:
```json
{
  "lifetimeDuration": "86400s",
  "iceServers": [
    {
      "urls": [
        "stun:74.125.247.128:3478",
        "stun:[2001:4860:4864:4:8000::]:3478"
      ]
    },
    {
      "urls": [
        "turn:74.125.247.128:3478?transport=udp",
        "turn:[2001:4860:4864:4:8000::]:3478?transport=udp",
        "turn:74.125.247.128:3478?transport=tcp",
        "turn:[2001:4860:4864:4:8000::]:3478?transport=tcp"
      ],
      "username": "CJez284GEgb6GwQdniQggKMFOJUBSLqElC5QlgE",
      "credential": "7HXq8E+q9GPqlHtw/ZPw+fYUTVQ="
    }
  ],
  "blockStatus": "NOT_BLOCKED",
  "iceTransportPolicy": "all"
}
```

Key observations:
- Token lifetime: 24 hours
- STUN servers: Google's public STUN at `74.125.247.128:3478`
- TURN servers: Same IP, UDP and TCP transports, with ephemeral credentials
- `iceTransportPolicy: "all"` — allows both direct and relayed connections

---

## SDP Exchange (Entry 24: `eyWvXc`)

### Request: SDP Offer
POST body contains the bundled SDP offer with ICE candidates:
```
f.req=[[["eyWvXc","[{\"sdp\":\"v=0\r\no=- 2930710343720394882 3 IN IP4 ...
```

The SDP offer includes:
- Audio: Opus codec (payload type 111), plus fallbacks (G722, PCMU, PCMA, CN)
- DataChannel: SCTP over DTLS (for `AgentCommsUserMessage` protobuf)
- ICE candidates: host, srflx (STUN), relay (TURN) — all bundled, no trickle
- Fingerprint: SHA-256 DTLS certificate

### Response: SDP Answer
Server returns bundled SDP answer with:
- Audio: Opus selected
- DataChannel: SCTP confirmed
- ICE candidates: Google relay servers at `10.4.64.58`, `10.4.64.57`
- BUNDLE: both media and data on same transport

---

## SDP Captures (_capture.jsonl)

Three entries in custom format:

| # | Direction | Type | Description |
|---|-----------|------|-------------|
| 0 | local | offer | Initial SDP without ICE candidates |
| 1 | local | offer | Bundled SDP with ICE candidates (host, srflx, relay) |
| 2 | remote | answer | Server SDP with Google TURN relay candidates |

Entry 1 shows the client's ICE candidates:
- `host`: mDNS-masked local addresses (`e77a91e7-b4c3-4ec1-9652-36b80d1ec363.local`)
- `srflx`: STUN-discovered public IP (`98.97.25.225:14315`)
- `relay`: TURN relay (`10.8.64.9:31197`)

Entry 2 shows the server's candidates:
- `host`: Google internal IPv6 (`fd14:988a:50ee:10c:b0:481:7446:c0e7`)
- `relay`: TURN relay at `10.4.64.58:10696`

---

## Signaler Long-Poll (signaler-pa.clients6.google.com.jsonl)

### chooseServer (entry 0)
Request (base64-decoded):
```json
[[null,null,null,[9,5],null,
  [["tailwind"],[null,1],
   [[["discoveredSource"],
     ["2ed71b32-63bb-4c22-a779-210d4f9bec5f"]]]]
],null,null,0,0]
```

Response:
```json
["iZ4qDiJaDO-B0ZhYxcsnmsvwUYMh0asNTthstPg51bE",3,null,
 "1775601597096113","1775601597096189"]
```

Returns a session token and timestamps. Service name is `"tailwind"` with the
notebook ID. The `[9,5]` may be protocol version numbers.

### multi-watch/channel (entries 1-5)
Long-poll channel with CORS preflight (OPTIONS) + POST subscription. Uses
Google's Channel API (`CVER=22`, `VER=8`, `gsessionid`, `SID`). This maintains
a persistent connection for real-time updates during the interactive session.

---

## GenerateNotebookGuide (Entry 15: `VfAZjd`)

Returns both a summary and suggested questions:

### Summary (position [0][0])
One-paragraph overview of notebook contents.

### Suggested Questions (position [0][1])
Array of `[display_text, briefing_doc_prompt]` pairs:
```json
[
  ["What services and features can be managed via these RPCs?",
   "Create a detailed briefing document designed to address..."],
  ["How does the system handle different types of document sources?",
   "Create a detailed briefing document designed to address..."],
  ["Explain the architecture of the NotebookLM API client implementation.",
   "Create a detailed briefing document designed to address..."]
]
```

Each suggestion includes both the user-facing question and the expanded prompt
that would be sent to generate a full briefing doc.

### Suggestion Chips with Type Codes (position [0][5])
```json
[["What services and features...", 9], ...]
```

---

## Audio Playback DataChannel Capture

42 messages captured during audio overview playback. All are incoming
(server → client) `AgentCommsUserMessage` protobuf over the WebRTC DataChannel.

### Protobuf Wire Structure

Every message is wrapped in a common envelope:

```protobuf
message DataChannelMessage {      // field 2 in wire format
  int32 sequence_number = 1;      // monotonically increasing
  int32 payload_size = 2;         // byte size of the inner message
  AgentCommsUserMessage msg = 4;  // the actual message
}
```

### AgentCommsUserMessage Field Mapping

Based on decoded captures, field 4 contains one of these sub-messages
(exactly one populated per message):

| Wire Field | Type ID | Name | Observed Data |
|------------|---------|------|---------------|
| `4.2` | 2 | AGENT_UTTERANCE | `{1: speaker_id, 2: "speakers", 3: "transcript text", 4: is_final, 5: "utterance_id", 8: ""}` |
| `4.3` | 3 | TTS_EVENT | `{1: event_type, 2: "utterance_id", 3: segment_idx, 4: {1: epoch_sec, 2: nanos}}` |
| `4.4` | 4 | SEND_AUDIO_EVENT | `{1: trigger_type, 2: "utterance_id"}` |
| `4.6` | 6 | PLAYBACK_EVENT | `{1: "state", 2: "state", 3: "state"}` — open/close/mute states |
| `4.7` | 7 | MICROPHONE_EVENT | `{1: status_code}` |

### Message Flow During Playback

```
Entry 0:  PLAYBACK_EVENT  → {6: {1: ""}}         (playback init)
Entry 1:  PLAYBACK_EVENT  → {6: {3: ""}}         (state update)
Entry 2:  MICROPHONE_EVENT → {7: {1: 3}}          (mic muted, status=3)
Entry 3:  AGENT_UTTERANCE  → {2: {1:2, 2:"Host Speaker,Expert Speaker", 3:"Have you ever tried..."}}
Entry 4:  AGENT_UTTERANCE  → (next transcript segment)
Entry 5:  TTS_EVENT        → {3: {1:1, 2:"utterance_id", 4:{1:epoch, 2:nanos}}}
...
Entry 33: AGENT_UTTERANCE  → {2: {1:2, 3:"", 4:1, 5:"utterance_id"}}  (final, is_final=1)
Entry 34: SEND_AUDIO_EVENT → {4: {1:1, 2:"utterance_id"}}  (trigger play)
Entry 35: SEND_AUDIO_EVENT → {4: {1:2, 2:"utterance_id"}}  (trigger stop)
Entry 36: TTS_EVENT        → {3: {1:2, 2:"utterance_id", 3:37, 4:{epoch}}}  (TTS complete)
Entry 41: PLAYBACK_EVENT   → {6: {2: ""}}         (playback ended)
```

### AGENT_UTTERANCE Detail (field 4.2)

```protobuf
message AgentUtterance {
  int32 speaker_id = 1;       // 2 = AI host (both speakers share this ID)
  string speakers = 2;        // "Host Speaker,Expert Speaker" (comma-separated)
  string transcript = 3;      // The spoken text for this segment
  int32 is_final = 4;         // 1 = last segment of this utterance
  string utterance_id = 5;    // Unique ID: "5932763328138406579"
  string unknown_8 = 8;       // Empty string
}
```

### TTS_EVENT Detail (field 4.3)

```protobuf
message TtsEvent {
  int32 event_type = 1;       // 1 = start, 2 = end
  string utterance_id = 2;    // Matches AgentUtterance.utterance_id
  int32 segment_index = 3;    // Which segment (for multi-part utterances)
  Timestamp timestamp = 4;    // {1: epoch_seconds, 2: nanos}
}
```

### SEND_AUDIO_EVENT Detail (field 4.4)

```protobuf
message SendAudioEvent {
  int32 trigger_type = 1;     // 1 = play, 2 = stop
  string utterance_id = 2;    // Matches AgentUtterance.utterance_id
}
```

### Transcript Content

The transcript segments contain the actual audio overview dialogue:

> "Have you ever tried to test an application that talks to a remote API,
> and, uh, only to have the tests just completely fail because the network
> blipped for like half a second."
>
> "Oh, yeah. Constantly."

Each segment is ~500-1000 bytes of transcript text, with speaker attribution
via the `speakers` field. The conversation alternates between the two AI hosts
discussing the notebook's source material.

---

## Interactive Audio DataChannel Capture

50 messages captured during interactive session (play → user speaks → agent
responds → resume playback). Both directions visible — all arrive as RECEIVE
since the JS monkey-patch captures incoming DataChannel messages.

### Complete Interactive Flow

```
 #0   PLAYBACK      → playback init
 #1-4 SEND_AUDIO    → audio triggers (playing overview)
 #5   PLAYBACK      → state change
 #6   AGENT_UTT     → "Oh, hey I think our listener's got something to say!"
 #7-8 TTS_EVT       → TTS start/end for agent utterance
 #9-14 SEND_AUDIO   �� audio triggers
 #15  MICROPHONE    → mic activated (user about to speak)
 #16  USER_UTT      → "Hey."
 #17  USER_UTT      → "Hey there."
 #18  USER_UTT      → "Hey, there. This is."
 ...                → (progressive real-time transcription)
 #27  USER_UTT      → "Hey there. This is a test message."
 #31  USER_UTT      → "Hey there. This is a test message. One two three."
 #32  MICROPHONE    → mic deactivated
 #33-34 STATUS      → status messages
 #35  AGENT_UTT     → (empty/partial — agent starting response)
 #36  AGENT_UTT     → "Well, hello there! Welcome, welcome! We love it when
                        you chime in. That is ab..."
 #37  TTS_EVT       → TTS start
 #38-48 SEND_AUDIO  → audio triggers (agent speaking)
 #49  PLAYBACK      → playback ends
```

### USER_UTTERANCE Detail (field 4.1)

```protobuf
message UserUtterance {
  string transcript = 3;       // Progressive real-time text
  int32 is_final = 4;          // 1 = final transcript for this utterance
  // Other fields TBD — need more captures with varied input
}
```

User speech arrives as a stream of progressive refinements:
```
"Hey."
"Hey there."
"Hey, there. This is."
"Hey there. This is a test message."
"Hey there. This is a test message. One two three."
```

Each new message replaces the previous — same pattern as the thinking chunks
in chat streaming (replacement, not cumulative).

### STATUS_MESSAGE Detail (field 4.8)

Observed between user speech end and agent response start. Small messages
(12-23 bytes). Likely connection state or processing indicators.

### Interactive Session vs Passive Playback Differences

| Aspect | Passive Playback | Interactive |
|--------|-----------------|-------------|
| USER_UTTERANCE | None | Progressive STT transcription |
| MICROPHONE events | 1 (muted) | 2 (activate + deactivate) |
| STATUS messages | None | 2 (between user/agent turns) |
| AGENT_UTT content | Full transcript segments | Shorter responses to user |
| Turn count | ~30 one-way segments | Mixed bidirectional |

---

## Actionable Findings

### For Chat Streaming (tasks #27-29)
1. **Thinking detection confirmed**: `**` prefix heuristic works
2. **Citations at `[1]`**: Confidence scores + char ranges + source excerpts
3. **Citation mapping at `[2]`**: Char ranges → source indices
4. **Follow-up chips at `[3][0]`** (strings) and `[5][0]` (with type codes)
5. **Conversation metadata at `[0][2]`**: `[conv_id, msg_id, seq_num]`

### For Audio Download (task #10)
- `sqTeoe` returned successfully with format catalog — audio-download may need
  a different RPC (maybe `VUsiyb` GetAudioOverview) to get the actual audio URL

### For Artifacts (task #12)
- `gArtLc` ListArtifacts returned 165KB successfully via direct RPC — the 400
  error was only from the orchestration service wrapper

### For WebRTC (tasks #36-38)
- Complete signaling flow captured: `Of0kDd` → `eyWvXc` → SDP exchange
- ICE servers, TURN credentials, and SDP structure all documented
- `signaler-pa` long-poll channel for real-time state sync

### For `e3bVqc` Polymorphism (task #32)
- Same RPC ID serves both DeleteChatHistory and GetConversations (full data)
- Need to diff the POST bodies to understand the arg format difference

### For Interactive Audio (tasks #36-38)
1. **Full signaling flow captured**: `Of0kDd` (ICE/TURN) → `eyWvXc` (SDP) → established
2. **DataChannel protobuf decoded**: All 7 message types observed and mapped
3. **USER_UTTERANCE is progressive replacement** (not cumulative like answer text)
4. **Turn-taking protocol**: MICROPHONE → USER_UTT stream → MICROPHONE → STATUS → AGENT_UTT → TTS_EVT → SEND_AUDIO
5. **Agent interruption**: Agent detects user intent before mic activates ("I think our listener's got something to say!")

### New RPC to Investigate
- `ub2Bae` — 412KB response, not in our mapping
