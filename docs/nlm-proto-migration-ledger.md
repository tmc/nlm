# NotebookLM proto migration ledger

This ledger records migration state from the live checkout. A family advances
only when each proof column has direct fixture or corpus evidence; a generated
binding alone is not proof.

The test-only `assertEquivalent` harness recursively reports phase, exact
field/index path, old value, new value, and mismatch count. It is strict by
default; family tests must perform any documented normalization before calling
it.

| RPC family | Wire proof | Semantic proof | Public API proof | CLI proof | Encoder proof | Live switch | Legacy deletion |
| --- | --- | --- | --- | --- | --- | --- | --- |
| Notes (`cFji9`, `CYK0Xb`, `cYAfTb`, `AH0mwd`) | **partial**: generated `GetNotesWireResponse` decodes the keyed fixture in `notes_proto_test.go`; complete observed-variant corpus still required | **pass (fixture)**: `TestGetNotesProtoAdapterMatchesLegacyParser` compares the public projection | **pass (unit)**: nil response and empty-entry behavior covered; live RPC now uses generated response | pending | pass: generated encoders are exercised by existing note encoder tests | **GetNotes switched**; create/mutate/delete were already generated | retained: `parseNotesResponse` remains the legacy oracle until corpus/deletion gates pass |
| Projects | **request proof pass** for `wXbhsf`; response corpus remains partial | unverified for the broader project family | **ListRecentlyViewedProjects behavior preserved**; other project methods mixed | unverified | **pass (live regression)**: `wXbhsf` encoder pins `[null, 1, null, [2]]` | **ListRecentlyViewedProjects uses generated encoder**; other project methods mixed | retained: project mutations and complete response corpus still need family-level proof |
| Sources and freshness | partial (typed freshness path) | partial | **live proto path** for freshness/refresh; raw fixture parsers retained as behavioral oracles | pending | **pass**: generated freshness and DeleteSources encoders are used by live methods; DeleteSources wire JSON is fixture-pinned | **freshness, refresh, and DeleteSources switched** | retained: `parseCheckFreshnessResponse`/`parseRefreshSourceResponse` remain fixture oracles until corpus deletion gates |
| Source guides (`tr032e`) | **fixture pass**: generated response decodes the keyed guide tuple | **pass (fixture)**: `sourceGuideFromProto` matches the legacy summary/topic projection | **pass (unit)**: empty and populated projections preserve public fields | pending | **pass (unit)**: generated request encoder matches the fixed context sentinel | **GenerateSourceGuide switched** | retained: legacy positional helper remains in tests as the comparison oracle |
| Labels | **fixture pass**: populated/empty reads and `[null, labels]` mutation responses decode through `beprotojson` | **pass (fixture)**: read and mutation adapters use the typed equivalence harness | **pass (unit)**: ordering, empty slices, and public `Label` projection preserved | compile-covered; CLI label commands still need output fixtures | **pass (fixture)**: GetLabels/CreateLabel/MutateLabelsMode/MutateLabel/DeleteLabels encoders match captured shapes | **GetLabels, CreateLabel, mode 0/1, rename, source attach, deletion switched**; GenerateLabels and emoji remain legacy | retained: `parseLabelsResponse` remains the oracle for empty-mode and emoji variants |
| Artifacts | partial (typed artifact paths) | unverified | legacy/proto mixed | **compile pass**: generated preview types are now consumed by `getArtifact`; output fixture equivalence still required | partial | legacy/proto mixed | retained |
| Notebook guides | **fixture pass**: typed response path retains the existing `VfAZjd` wire decoder | **pass (unit)**: generated response is returned unchanged | **pass (existing record tests)**: public response and errors are unchanged | compile-covered | **pass (unit)**: generated request encoder matches the fixed context sentinel | **GenerateNotebookGuide switched** | retained: legacy custom encoder remains only in the internal-method compatibility tests |
| Sharing | **fixture pass**: `ProjectDetails` wire fixture decodes through `beprotojson` | **pass (fixture)**: `TestProjectDetailsProtoAdapterMatchesLegacyParser` compares owner/public projection | **pass (unit)**: public `OwnerName`/`IsPublic` projection and flag fallback covered; mutations unchanged | pending | pass: generated `GetProjectDetails` encoder is used by the live method | **GetProjectDetails switched**; sharing mutations remain legacy/proto mixed | retained: share mutation parsers still require URL/UUID normalization |
| Account state | partial: generated `GetOrCreateAccount` path exists; compact `GetAccountStatus` shape is distinct | unverified for compact status | generated account API plus legacy status API | pending | generated account encoder path exists | mixed: `GetOrCreateAccount` proto, `GetAccountStatus` legacy | retained: compact status parser is not interchangeable with `Account` |
| Audio/video results | **request pass**: GetAudioFormats sentinel matches captured corpus shape; result variants still need typed comparison | unverified for audio/video result adapters | legacy result projection; GetAudioFormats public behavior unchanged | pending | **pass (fixture)**: generated GetAudioFormats encoder pins the captured sentinel | **GetAudioFormats request switched**; create/get/delete audio and video paths remain mixed | retained: result parsers and polymorphic create responses need corpus-backed adapters |
| Analytics | **blocked**: observed AUrzMb is metric-series data, while static `ProjectAnalytics` models scalar counts; `TestAnalyticsProtoModelIsNotSeriesModel` locks this mismatch | blocked pending corrected proto shape and adapter | legacy | pending | pass: generated request encoder exists but response model is wrong | legacy | retained with evidence: do not decode series as scalar account metrics |
| Conversation history | **fixture pass (list)**; message history remains unverified | **pass (fixture, list)**: generated conversation references preserve ID ordering | **delete and list behavior unchanged**; history API remains legacy | unverified | **pass (unit)**: GetConversations and DeleteChatHistory encoders match captured shapes | **GetConversations and DeleteChatHistory switched**; conversation history remains legacy | retained: large polymorphic history response still lacks corpus-backed adapter proof |
| Deep research | unverified | unverified | **delete behavior unchanged**; session APIs remain legacy | unverified | **pass (fixture)**: generated DeleteDeepResearch encoder matches the 4-position LBwxtb capture | **DeleteDeepResearch switched**; session reads/start/import remain legacy | retained: polymorphic session responses and bulk-import variants still require corpus-backed adapters |
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
