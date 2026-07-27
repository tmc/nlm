# Rich types and protobuf alignment — gap analysis & remediation plan

Status: proposed · Owner: (assign) · HEAD at time of writing: `c25326a4`

## Purpose

This document is the authoritative record of the gaps a Go-team panel review found in
`nlm` at `c25326a4`, framed around a single non-negotiable constraint:

> **We must preserve the hard-won protobuf alignment and carry rich types end-to-end.
> No fix in this plan may regress the wire modeling, drop the rich span tree, or
> reintroduce a lossy flat-text path.**

The wire modeling — 3723 lossless corpus records, the renumbered `ChatMessage`
proto, the named orchestration fields, the `RichDocument`/`Span` tree — was expensive
to earn. The review surfaced one place where a migration *already* threw part of it
away (the streaming rich tree, §1) and several places where the codebase carries two
representations of the same thing. The through-line of this plan is **convergence on
the generated protobuf types**, deleting hand-rolled parallel models as we go, never
the reverse.

The panel filed 39 findings; 34 survived independent file-level verification. This
document keeps the ones that touch rich types, the wire, and the public API shape,
plus the structural cleanups that must be sequenced around them. Each item cites a
real `file:line` verified against the tree.

---

## 0. The rich-type architecture as it stands today

There are **two decode paths** producing **one shared renderer**. Understanding this
is the whole key to the plan.

```
  NOTES  ──►  *pb.RichDocument  ─────────────┐
             (proto, api.Note.Rich,          │   richDocumentFromProto()
              client.go:43)                   │   cmd/nlm/rich_bridge.go:36
                                              ▼
                                        *richDocument  ──► projectRichDocument()
                                        (cmd/nlm/rich_        cmd/nlm/rich_document.go:173
                                         document.go:42)      → HTML / Markdown
                                              ▲
  CHAT   ──►  *api.RichContent  ─────────────┘   richDocumentFromAPI()
             (hand-rolled positional            cmd/nlm/rich_bridge.go:23
              decoder, rich_content.go:29,
              decodeRichContentFromSegment
              at rich_content.go:106)
```

**Good news the review confirms:** the *renderer* is already unified. `rich_bridge.go`
has both `richDocumentFromProto(*pb.RichDocument)` (`:36`) and
`richDocumentFromAPI(*api.RichContent)` (`:23`), both landing on the same internal
`*richDocument` and the same `projectRichDocument()` projection. We are **not** asked
to write a new renderer.

**The fracture is on the decode side.** Notes ride the generated proto
`RichDocument`/`Span`/`SpanGroup`/`ListItem`/`TextMarks` types straight from the wire.
Chat hand-rolls an equivalent `RichContent`/`RichSpan`/`RichListItem` model
(`rich_content.go`) with its own positional decoder. The two model the *same wire
tree*. That duplication is the debt; proto is the side of it we keep.

**Second structural fact:** this entire renderer — ~4,700 lines of it — lives in
`package main` under `cmd/nlm`, not in an importable package (see §8). So the rich
types are both *fractured* (two decoders) and *trapped* (unimportable). §1 unifies the
decoders on proto; §8 lifts the unified renderer into a real package. The two are the
same cleanup viewed from the type side and the package side.

---

## 1. BLOCKER-CLASS BUG — streaming drops the rich span tree (finding #1)

**Severity: high (behaviorally a regression). This is item zero; nothing else moves first.**

### What's wrong

`extractChatPayload` (`internal/notebooklm/api/client.go:4938`) has two branches:

- **proto branch** (`client.go:4941`, taken when `beprotojson.Unmarshal` succeeds — i.e.
  the *normal* live path) builds `chatPayload{Text, Citations, FollowUps, wirePhase}`
  and **never assigns `payload.Rich`.**
- **legacy branch** `extractChatPayloadLegacy` (runs *only* when proto decode fails)
  sets `p.Rich = decodeRichContentFromSegment(inner)` at `client.go:5259`.

So on every *successful* decode — which is the steady state in production — `payload.Rich`
is `nil`. It propagates to `ChatChunk.Rich` (`client.go:4688`), the HTML renderer's
`shouldReflowFromTree(m.Rich, …)` sees `nil` (`cmd/nlm/chat_render_html_answer.go:139`),
and the answer is **flat-reflowed instead of walked as a span tree**. The
`779e13a7` migration ported text + citations to proto but dropped the rich tree.

This is the exact shape the review flagged as the dominant defect: *a completed
migration must leave no second path that is the only one still doing the real work.*
Here the only path that produces `Rich` is the failure path.

### The fix — and why it advances proto alignment instead of patching around the hole

The naive fix is "call the positional `decodeRichContentFromSegment` from the proto
branch too." **Do not do that.** It re-entrenches the hand-rolled `RichContent`
decoder we want to retire.

The aligned fix: the decoded proto response already contains the tree. `ChatAnswer`
has `GetDocument() *pb.RichDocument` (`gen/notebooklm/v1alpha1/orchestration.pb.go:6392`,
document is field 5 of the answer). Populate the rich tree **from the proto** in the
proto branch.

Two staged options, in order of preference:

- **1a (target state):** retype the carrier to proto. `chatPayload.Rich` and
  `ChatChunk.Rich` become `*pb.RichDocument` (matching `api.Note.Rich` at
  `client.go:43`). The proto branch assigns `payload.Rich = generated.GetAnswer().GetDocument()`.
  The renderer switches from `richDocumentFromAPI` to `richDocumentFromProto` — a
  function that **already exists**. The hand-rolled `api.RichContent`, `RichSpan`,
  `RichListItem`, `decodeRichContentFromSegment`, and `richDocumentFromAPI` become
  dead and are deleted in a follow-up commit. This is the full convergence: chat and
  notes both ride `pb.RichDocument`.

- **1b (bridge, if 1a is too large for one change):** keep `*api.RichContent` as the
  carrier type but build it *from the proto* — add `richContentFromProto(*pb.RichDocument) *api.RichContent`
  and call it in the proto branch. This still eliminates the drop and keeps the
  positional decoder only as the legacy-fallback path. It leaves the duplication in
  place, so it is strictly a stepping stone to 1a, not the destination.

Prefer **1a.** It deletes code, unifies the two paths on the generated types, and is
the literal embodiment of "don't lose the protobuf alignment."

### Mandatory test (regression lock)

Add a table test in `internal/notebooklm/api` that feeds a **proto-decodable**
streaming frame (one that makes `beprotojson.Unmarshal` succeed) and asserts
`payload.Rich != nil` **and** that the projected block tree is non-trivial (more than
one block, or a list coalesced). A test that only checks `!= nil` would pass on an
empty document. Pin at least one list case and one inline-mark case so the
`ListItem`/`TextMarks` carry-through is covered. This test is the guard that stops the
next migration from silently dropping the tree again.

### Corpus / alignment guard

Before and after: run the corpus lossless check (betool `--verify`) on the streaming
records to confirm the proto decode still round-trips. **Note the standing hazard**
(see project memory): the traffic corpus was under `/tmp/nlm-traffic`, which macOS
sweeps nightly, so the gate may need re-recording to `~/tmp` first. Do not report
"corpus lossless" as green without actually running it against real records — the
migration this bug lives in is exactly one where that gate was never run.

---

## 2. No `context.Context` on the library surface (finding #2)

**Severity: high. The single biggest API-shape defect.**

No network method on `api.Client` takes a `context.Context`. Every HTTP call goes
through `http.NewRequest` (never `NewRequestWithContext`) at `client.go:250, 1087,
1169, 2491, 4409`, and orchestration calls synthesize `context.Background()` **41
times**. The tell is the single `GetProjectWithContext` (`client.go:5931`) sitting
beside the context-free `GetProject` (`client.go:392`) — a migration begun and
abandoned. A chat stream can block for the 5-minute `chatInitialResponseTimeout`
(`client.go:4095`) with **no way for the caller to cancel**.

**Fix:** add `ctx context.Context` as the first parameter to every I/O method; thread
it into `http.NewRequestWithContext` and the generated service calls; collapse
`GetProjectWithContext` back into `GetProject`. This is a large, mechanical signature
churn (cost L). Sequence it carefully (see §9) so it does not collide with the config
consolidation (§4) — both touch every method signature, and you want to touch each
signature once.

**Alignment note:** context threading does not touch the wire encoding or the proto
types. It is pure call-shape. Land it as its own reviewable change; do not fold wire
changes into it.

---

## 3. Two package-global mutable registries (finding #4)

**Severity: medium. Small, self-contained, unblocks §4.**

- `beprotojson.SetGlobalDebugOptions` (`internal/beprotojson/beprotojson.go:192`)
  mutates the package var `defaultUnmarshalOptions` (`:187`) with no lock, flatly
  contradicting `doc.go`'s "API-compatible with `protojson`" claim (`protojson` has no
  global mutator). Its sole caller is `cmd/nlm/main.go:121`. **Fix:** delete it; pass
  debug through the already-existing `beprotojson.UnmarshalOptions{DebugParsing: true}.Unmarshal(...)`.
  This keeps the codec instance-configured and preserves the protojson-mirroring
  design the review praised.

- `batchexecute.AddErrorCode` (`internal/batchexecute/errors.go:520`) is an
  unsynchronized write to the package-global `errorCodeDictionary` (`:103`) read from
  request goroutines via `GetErrorCode` (`:335`). It is **dead in the tree**. **Fix:**
  unexport the static table (or, only if runtime extension is genuinely wanted, guard
  it with a `sync.RWMutex`).

**Alignment note:** neither change affects decode fidelity — `SetGlobalDebugOptions`
only toggles debug logging, not parsing behavior. Verify that with a before/after
corpus check anyway, since it lives in the codec package.

---

## 4. Configuration scattered across four mechanisms (cross-cutting theme)

**Severity: medium (structural). Do before §2.**

`api.Client` is configured by functional options **and** mutable `Set*` setters
(`SetUseDirectRPC` `client.go:226`, `SetDebug` `:230`, `SetAuthUser` `:235`) **and** a
constructor reading `os.Getenv("NLM_DEBUG")` twice (`:207, :220`) **and** two
package-global mutators (§3). Worse, `SetDebug` writes `c.config.Debug` but never
propagates to `c.rpc.Config.Debug` — debug is fractured across two non-communicating
paths (a known footgun in project memory: the debug-propagation note).

**Fix:** fold everything into `With*` options applied once in `New`. Replace
`api.New(authToken, cookies string, …)` — two same-typed adjacent strings, a silent
swap footgun (`client.go:203`) — with a `Credentials` struct. Remove the double
`os.Getenv` read. Make `SetDebug` (or `WithDebug`) set both `config.Debug` and
`rpc.Config.Debug` so debug actually reaches the RPC layer.

Do this **before** the context change so the constructor and method signatures are
touched once, not twice.

---

## 5. Package renames that the compiler is already complaining about (finding #5)

**Severity: high for the collisions, low for the cosmetic one.**

- `internal/sync` declares `package sync`, **shadowing the stdlib** and forcing an
  `nlmsync` import alias at every site (`cmd/nlm/main.go:30`, `sync_pack.go:7`,
  `cmd/nlm/source_flags.go:12`). Make the alias the real name: `internal/nlmsync`,
  `package nlmsync`. The alias is the compiler telling you the name is wrong.

- `internal/rpc/argbuilder` is the only package under a bare `internal/rpc/` parent,
  while the real rpc package lives at `internal/notebooklm/rpc`. Move it to
  `internal/argbuilder`, delete the empty parent. **83 importers, all generated** —
  update the generator template, don't hand-edit the generated files.

- Module-root `package proto` (`proto/gen.go:14`) shadows
  `google.golang.org/protobuf/proto`, which 52 files import. Rename to `package
  protobuild` **before** a future edit in that package trips the shadow.

- **Deferred:** `internal/notebooklm/api` → `package notebooklm` (`client.go:1`). Real
  nit (`api` carries no meaning) but ~1044 call sites of pure cosmetics. Not now.

**Alignment note:** these are mechanical renames. The `argbuilder` move touches the
**codegen template** — regenerate and diff the generated output to confirm
**zero functional drift** (buf-regen zero-drift discipline, per project memory). Renames
must not change a single generated byte beyond the import path/package clause.

---

## 6. `internal/notebooklm/api` — the crown jewel, the weakest-documented surface (finding #6)

**Severity: medium.**

This package is the entire public library, the 6935-line monolith, and ~80 of
`Client`'s 96 exported methods have **no doc comment** — `ListRecentlyViewedProjects`
(`client.go:270`), `CreateProject` (`:281`), `GetProject` (`:392`), `DeleteProjects`
(`:409`), `MutateProject` (`:422`), `DeleteSources` (`:621`), `MutateSource` (`:656`),
`RefreshSource` (`:681`), and the rest sit under a blank line or a bare section banner.

**Fix:** give each a name-led first sentence per <https://go.dev/doc/comment>, demoting
existing wire-protocol notes to a second paragraph (keep the honest `TODO(har)` wire
comments — the review explicitly called those out as *patterns to keep*). Add
`internal/method/doc.go` (no package comment exists on any of its 13 files) and a
`// Command exporthttprr …` header (`internal/cmd/exporthttprr/main.go:1`).

**Do this near-last** (§9), so you document the surface you are *keeping*, not one you
are about to reshape with §2/§4.

---

## 7. Dead code + a deadcode gate (finding #7)

**Severity: low, but do it early — before file splitting (§8).**

- `internal/auth/signaler.go` — unreachable from `main` (has a live test file; delete
  file + test together or neither).
- `internal/auth/refresh.go` — `StartAutoRefresh` (`:336`), `RefreshLoop` (`:306`),
  `DefaultAutoRefreshConfig` (`:327`) orphaned by `TokenManager.StartAutoRefreshManager`.
- The **36 dead non-options wrappers** (e.g. `parseGenerateChatArgs`
  `cmd/nlm/chat_flags.go:139`) plus `packageGlobalOptions()` (`cmd/nlm/cli_parser.go:432`)
  — delete as part of finishing the `WithOptions` migration.

**Verify each with `deadcode -tags integration` first** — the repo's own
`.github/copilot-instructions.md:45` warns integration-tagged tests can hide usage.
Then add a deadcode CI gate (the repo currently has **no test/lint CI** — see §10).

**Alignment note:** confirm none of the "dead" wrappers are the only caller of a wire
path before deleting. Run the full suite after each deletion.

---

## 8. Extract logic out of `cmd/nlm` into well-formed packages (structural — the spine)

**Severity: medium-high (package design). This is the largest structural change and the
one the topology lens should have caught. Do the client-side split first, then the
extraction, then main.go shrinks as a consequence.**

### The problem, quantified

`cmd/nlm` holds **21,201 lines of non-test code**, and most of it is not CLI glue — it
is genuine, testable, reusable *logic* trapped in `package main` where nothing can
import it, isolate it, or reuse it (e.g. from the MCP server in `internal/nlmmcp`, or a
future library consumer). The `main` package should be a thin CLI shell: flag parsing,
dispatch, and I/O wiring. Today it is a library with a `func main()` stapled on.

The trapped concerns, by cluster:

| Cluster | Files (LOC) | Real dependencies | Home package |
|---|---|---|---|
| **Rich rendering** | `chat_render_html*.go`, `chat_render_markdown.go`, `note_render_html.go`, `note_render_markdown.go`, `rich_bridge.go`, `rich_document.go`, `chat_document.go`, `note_document.go`, `markdown_subset.go`, `citation_math.go` (~4,700) | only `api` + `pb` — several import **nothing** from the module | `internal/richrender` (or `internal/notebooklm/render`) |
| **betool inference** | `betool.go`, `betool_infer.go`, `betool_delta.go`, `betool_corpus.go`, `betool_text.go` (~2,800) | wire/proto only | `internal/betool` (or fold toward the `beproto` shim once that lands) |
| **Citation resolution** | `resolve_citations.go`, `source_match.go` (~530) | `api` | `internal/richrender` or `internal/citations` |
| **Source ingestion helpers** | `add_sources.go`, `source_read.go`, `source_flags.go` bodies (~1,300) | `api` | fold into `internal/notebooklm/api` or a `sources` subpackage |

**Evidence this is clean to do:** the render files import only `api` and `pb` — no
`flag`, no `cobra`, no CLI machinery. `rich_document.go`, `chat_render_html_answer.go`,
`markdown_subset.go`, `citation_math.go` import *nothing* from the module: they are pure
functions on local types. The only coupling is to `api`, and that points the right way
— a renderer *should* depend on the client's types, not vice versa. There is no import
cycle risk as long as `api` does not depend back on the render package (it does not, and
must not).

### The move

1. **Create `internal/richrender`.** Move the rendering cluster there. Export the entry
   points the CLI needs (`RenderChatHTML`, `RenderNoteHTML`, `RenderChatMarkdown`, …).
   The internal `richDocument`/`richSpan` model and `projectRichDocument` become the
   package's private core; `richDocumentFromProto`/`richDocumentFromAPI` become its
   two public constructors. **This is where §1 pays off:** once chat carries
   `*pb.RichDocument` (option 1a), the package needs only the *proto* constructor and
   the `api.RichContent` constructor can be dropped entirely — the render package ends
   up proto-native.
2. **Create `internal/betool`** (or align with the landing `beproto` shim — coordinate
   with the beproto-integration branch owner; see project memory). Move the inference
   cluster. This is operator tooling; giving it a real package makes it testable with
   real fixtures instead of frozen copies.
3. **Shrink `cmd/nlm`** to dispatch + flag structs + thin command handlers that call the
   new packages. `main.go` (5035 lines) and `commands.go` should be dominated by the
   command table and argument wiring, not rendering or inference bodies.

### The client.go split (subsumed here)

`internal/notebooklm/api/client.go` (6935 lines) is the *other* monolith. Split it
along the seams that already exist in the function grouping: `client_chat.go`,
`client_audio.go`, `client_video.go`, `client_artifact.go`, `client_upload.go`,
`citations.go` (note the orphan `deep_research_test.go` with no `deep_research.go` — a
ready-made seam). This is a **pure file move within one package — zero behavior
change.** Do it first among the structural steps so the surface you then extract
against is legible.

### Discipline

- **Pure moves, one cluster per commit.** No behavior change in an extraction commit.
  Run `go build ./... && go test ./...` before and after; diff the exported symbol set
  to prove only locations changed.
- **Do this after** dead code is gone (§7) and signatures are stable (§2, §4), so you
  move less and don't relocate code you're about to delete or reshape.
- **Package naming** follows the house rules the review praised (no `utils`, no stutter,
  descriptive): `richrender`, `betool`, `citations` — not `render`-generic or
  `cmdutil`.
- **Preserve the `api` → render dependency direction.** If an extraction tempts you to
  make `api` import the render package, stop — that inverts the layering. Pass data
  out, render outside.

---

## 9. Localized symbol fixes (finding #8)

**Severity: mixed, mostly low. Finishing polish (§ order: with docs).**

- Lowercase dead exports in `package main`: `ChatSession` (`main.go:55`), `ChatMessage`
  (`:68`, and rename to `storedMessage` — it collides with `api.ChatMessage`),
  `AuthOptions` (`auth.go:34`).
- `GetAudioBytes` (`client.go:2061`) → `AudioBytes` (pure accessor, not an RPC; Go
  reserves the `Get` prefix against accessors).
- Lowercase the capitalized error at `client.go:2529` **preserving the load-bearing
  "browser authentication" substring** that `main.go:5002` matches on — this is a
  behavioral coupling; grep for the match before editing.
- Drop the empty exported `beprotojson.MarshalOptions` struct (`beprotojson.go:15`) if
  nothing configures marshaling; otherwise leave a doc note on why it's a placeholder.

---

## Order of operations (the sequencing contract)

Rationale: **fix behavior before structure, delete before refactor, and sequence the
two large signature edits (config, context) so they never overlap** — then cosmetic
renames and docs last against a stabilized shape. Every step is independently
buildable, testable, and (where it has an origin) pushable.

1. **§1 — Restore the rich span tree (proto-aligned, option 1a).** The only
   user-visible fix. Do it before anything moves `extractChatPayload`. Land the
   regression-lock test. Run the corpus guard.
2. **§7 — Delete dead code.** Verify with `deadcode -tags integration`; split less later.
3. **§3 — Retire the global mutators.** Small; unblocks config work.
4. **§4 — Consolidate configuration into options + `Credentials`.** Before context, so
   signatures are touched once.
5. **§2 — Thread `context.Context`.** The big signature churn; after config.
6. **§5 — Rename packages** (except the deferred `api` rename). Mechanical; regenerate
   the `argbuilder` template and diff for zero drift.
7. **§8 — Extract `cmd/nlm` logic into packages + split `client.go`.** The structural
   spine. Split `client.go` in place first (pure moves within `api`), then extract the
   render + betool clusters into `internal/richrender` / `internal/betool`, shrinking
   `main.go` to a thin CLI shell. Once §1 is in, the render package ends up
   proto-native. One cluster per commit, no behavior change.
8. **§6 + §9 — Documentation pass + remaining symbol nits.** Polish against the final
   shape — including the new package doc.go files for `richrender`/`betool`.

## Invariants that gate every step (do not violate)

- **Proto alignment is preserved.** No hand-rolled model may *replace* a generated
  proto type; convergence runs proto-ward only. The §1 fix must *remove* a hand-rolled
  path, not add one.
- **The rich tree survives end-to-end.** After §1, the streaming happy path carries a
  non-nil rich tree, verified by test, and the renderer walks it.
- **Corpus stays lossless.** Any change in `beprotojson`, `batchexecute`, `proto/`,
  `gen/`, or the chat decode path is gated by a real betool `--verify` run against real
  records — re-record traffic to `~/tmp` first if the corpus is missing; never report
  the gate green without running it.
- **Generated code is regenerated, never hand-edited.** The `argbuilder` move and any
  proto change go through the generator; diff for zero unexpected drift.
- **Full suite green after every commit.** `go build ./... && go vet ./... && go test ./...`,
  `gofmt -l` clean on tracked files.
- **Signed commits, atomic, house style.** One concern per commit; Go-style messages;
  no "Claude Code" attribution. Sign (ssh key) — stop if `ssh-add -l` is empty.

## Coverage caveat from the review

Two of the eight review lenses did not produce verified findings: **topology**
(package layering) failed its output contract, and **consistency** (cross-package
divergence) returned a stub. Those areas are *un-reviewed, not passed*. If a targeted
re-run surfaces layering findings, append them here as §11+ rather than reopening the
sequencing above.
