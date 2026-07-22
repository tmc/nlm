# NotebookLM proto migration ledger

This ledger records migration state from the live checkout. A family advances
only when each proof column has direct fixture or corpus evidence; a generated
binding alone is not proof.

| RPC family | Wire proof | Semantic proof | Public API proof | CLI proof | Encoder proof | Live switch | Legacy deletion |
| --- | --- | --- | --- | --- | --- | --- | --- |
| Notes (`cFji9`, `CYK0Xb`, `cYAfTb`, `AH0mwd`) | **partial**: generated `GetNotesWireResponse` decodes the keyed fixture in `notes_proto_test.go`; complete observed-variant corpus still required | **pass (fixture)**: `TestGetNotesProtoAdapterMatchesLegacyParser` compares the public projection | **pass (unit)**: nil response and empty-entry behavior covered; live RPC now uses generated response | pending | pass: generated encoders are exercised by existing note encoder tests | **GetNotes switched**; create/mutate/delete were already generated | retained: `parseNotesResponse` remains the legacy oracle until corpus/deletion gates pass |
| Projects | unverified | unverified | legacy | unverified | partial | legacy/proto mixed | retained |
| Sources and freshness | partial (typed freshness path) | partial | legacy/proto mixed | unverified | partial | legacy/proto mixed | retained |
| Labels | **fixture pass**: populated and empty `GetLabelsResponse` decode through `beprotojson` | **pass (fixture)**: `TestGetLabelsProtoAdapterMatchesLegacyParser` | **pass (unit)**: public `Label` ordering and empty-slice behavior preserved | compile-covered; CLI label commands still need output fixtures | pass: generated `GetLabels` encoder is exercised through the service binding | **GetLabels switched**; mutations remain legacy | retained: `parseLabelsResponse` remains the oracle for mutation variants |
| Artifacts | partial (typed artifact paths) | unverified | legacy/proto mixed | **compile pass**: generated preview types are now consumed by `getArtifact`; output fixture equivalence still required | partial | legacy/proto mixed | retained |
| Sharing | partial (typed sharing paths) | unverified | legacy/proto mixed | unverified | partial | legacy/proto mixed | retained |
| Account state | unverified | unverified | legacy | unverified | unverified | legacy | retained |
| Audio/video results | unverified | unverified | legacy | unverified | partial | legacy/proto mixed | retained |
| Analytics | unverified | unverified | legacy | unverified | pass (generated request) | legacy | retained |
| Conversation history | unverified | unverified | legacy | unverified | partial | legacy | retained |
| Deep research | unverified | unverified | legacy | unverified | partial | legacy | retained |
| Source text | unverified | unverified | legacy | unverified | partial | legacy | retained |
| Streaming chat | transport-only proof exists; typed response migration not attempted | unverified | legacy streaming behavior | partial | partial | legacy streaming parser | retained |

## Notes-family evidence

- The public method `Client.GetNotes` calls the generated service binding and
  adapts `GetNotesWireResponse` through `notesFromWireResponse`.
- The adapter intentionally projects only the fields exposed by the old
  positional parser (`note_id`, `content_text`, `title`, and `rich_text`). This
  preserves public behavior while the wire model retains metadata and type
  positions for later widening with an explicit API decision.
- `parseNotesResponse` is deliberately retained as a comparison oracle. It is
  not called from normal operation and must not be deleted until observed
  payload variants, request captures, CLI behavior, and the complete corpus
  satisfy the deletion gates in the migration goal.

## Current artifact blocker

The generated `Artifact` decoder is not yet a drop-in public adapter for the
legacy artifact family. On the captured slide artifact fixture, it populates
`title` and type-specific preview fields that `parseArtifactFromResponse`
intentionally leaves empty, while the legacy parser also promotes the state to
READY when it finds a rendered download URL. Switching without an explicit
projection would change public values and state semantics, so artifacts remain
on the legacy implementation pending a typed adapter and URL/state equivalence
fixtures.
