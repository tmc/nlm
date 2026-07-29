---
title: Citation Rendering Design
date: 2026-07-21
---

# Citation Rendering Design

How `nlm` should present citation data — across the terminal, Markdown export,
and rich HTML. This spec fixes the data model first (grounded in the decoded
wire), then derives a rendering for each surface from that one model. It exists
because the current renderer grew reactively during a bug-fix arc and encodes at
least one assumption the wire contradicts.

## Status

- The citation **data model** and parser are shipped and wire-verified
  (`notebooklm`, branch `work/citation-excerpts`). `Citation`
  carries per-marker and per-source fields; see below.
- The **renderer** (`cmd/nlm/main.go`, `renderCitationList` and friends) is the
  subject of this spec. Renderer correctness (§8 step 1) is **done**
  (commit `f1fcb3ea`): confidence is per-source, offsets are labeled, source
  spans render.
- **Markdown and HTML** surfaces are **shipped** (commit `107cd0f5`, hardened by
  the adversarial-review fixes in `e4dbfb64`).
- **Persistence (§9)** is the next gap: replay re-fetches titles live and loses
  them on stale auth. Fix is to save everything the renderers read so replay is
  offline and auth-independent.
- **TUI reader mode** (§7) and **rich-content answer rendering** (§10) are
  **proposed**, not built.

## 1. The model (ground truth)

Every field is decoded from the real
`LabsTailwindOrchestrationService.GetConversationHistory` frame
(`khqZz` / `GetConversationHistoryResponse`), verified with
`nlm betool decode-response --proto`. The proto's own field names settle what
belongs to what.

A citation joins **a claim in the answer** to **a passage in a source**. On the
wire that relationship is split across two structures:

```
marker record      { range: {start,end},  source_indices: [i, j, ...] }
per-source grounding[i]  { source:{source_id},
                           score,                 // confidence
                           reply_spans:[{start,end}],   // ANSWER offsets
                           source_spans:[{start,end, leaf.text}] }  // SOURCE offsets + excerpt
```

`Citation` (in `notebooklm/client.go`) is one **(marker, source)
pair** flattened from that:

| Field                     | Meaning                                         | Scope    |
|---------------------------|-------------------------------------------------|----------|
| `SourceIndex`             | the `[N]` marker in the answer text             | marker   |
| `StartChar` / `EndChar`   | offset of the claim **in the answer** (`reply_spans`) | marker   |
| `SourceID` / `Title`      | which source backs the marker                   | source   |
| `Confidence`              | grounding score 0–1 (`citationData[srcIdx][2]`) | **source** |
| `Excerpt`                 | verbatim cited text from the source             | source   |
| `SourceStart` / `SourceEnd` | offset of the excerpt **in the source doc** (`source_spans`) | source   |

### Two facts that drive every rendering decision

1. **A marker usually cites several sources.** In the reference frame, **35 of
   48 markers (73%) cite multiple sources**, up to 5. `[1]` covers answer
   115–241 and cites four sources.

2. **Confidence is per-source, and the marker record has no score at all.** When
   a marker cites four sources, there are four independent scores, and they
   differ (e.g. 0.91 / 0.87 / 0.71 / 0.68). Only `StartChar/EndChar` (the answer
   span) is truly per-marker; source, excerpt, source-span, **and confidence**
   are all per-source.

### Two offsets, never conflate them

There are two character ranges per citation and they index **different
documents**:

- `StartChar/EndChar` → the **answer** text (where the `[N]` claim sits).
- `SourceStart/SourceEnd` → the **source** document (where the excerpt lives).

The original citation bug was slicing a *source* body with *answer* offsets.
The renderer must never print a bare `chars N-M`: it must say which document.

## 2. Problems in the current renderer

`renderCitationList` groups by `SourceIndex` and calls `citationGroupHeader`,
which hoists confidence and span onto the marker header via `uniformConfidence`
/ `uniformSpan` — showing each only when all of a marker's sources agree.

- **Confidence is mis-placed.** It is per-source, so the header can only honestly
  carry it for single-source markers. For the 73% multi-source case
  `uniformConfidence` returns `!ok` and the confidence column **silently
  disappears** — losing the number exactly when there are several sources to
  tell apart. Compact mode then shows no scores at all.
- **The offset label is ambiguous.** `[chars 42-205]` gives no cue that it
  indexes the answer, and now that `SourceStart/End` exist, "chars" spans two
  documents.
- **`SourceStart/SourceEnd` are captured but never rendered** in the human view
  (they appear only in `--citations=json`).

## 3. Design principles

1. **One model, N renderers.** Spans, per-source rows, confidence, and excerpts
   are the same struct that feeds `--citations=json`. Every surface is a
   projection of it; none invents structure.
2. **Confidence lives on the source row, never the marker.** In every surface.
3. **Label the axis, always.** Never a bare `chars N-M`. Write `answer 42–205`
   and `src 965670–966914`. Two documents, two labels. Prefer a resolved
   `file:line` locator (via `--resolve-citations`) when the source is a txtar
   archive, with the raw `src N–M` as fallback.
4. **The spine follows the question.** The scan view (no excerpts) is
   marker-first: "what backs `[3]`?" The audit view (`--citation-excerpts`) is
   source-first: "what did this source actually say?" — because excerpt and
   source-span are per-source, and a source grounding several markers should
   appear once, not repeated.
5. **Confidence is a reading signal.** A score below a threshold (~0.75) renders
   amber, so weak grounding reads at a glance rather than requiring arithmetic.
   This is semantic color, distinct from any accent.
6. **Degrade honestly.** Every surface has a plain fallback: piped output drops
   color and links; an unsupported terminal drops OSC-8 and styled underlines to
   bracketed text. Never emit an escape the terminal will print as garbage.

## 4. Surface: TUI (streaming, today's printer)

`nlm` streams ANSI to a writer gated by `isTerminal()`. These are enrichments
to that stream — no event loop, degrade to plain text when piped.

**Scan view (default, marker-first, per-source confidence):**

```
Citations
  [1] answer 115–241
      p=0.91 codex:B149    apple
      p=0.87 7A59          Skill triage
      p=0.71 claude:E347   coordinator     ← amber
      p=0.68 codex:E2E8    jaccl native    ← amber
  [3] answer 1244–1254
      p=0.95 codex:B149    apple
```

- The marker heads its group with just the **answer span** (labeled).
- One row per source, its **own** confidence first (amber below threshold).
- `[N]` and each source handle are **OSC-8 hyperlinks** where supported; source
  handle opens the source, `[N]` jumps to it. Fall back to bracketed text.
- Cited answer spans get a **styled underline** in the streamed answer itself
  (solid = grounded, wavy = weak) — the inline half of the model.

**Audit view (`--citation-excerpts`, source-first):**

```
Citations  · grounds [1][3]
  7A59  Skill triage   src 965670–966914   (or file:line if resolved)
    [1] p=0.87 answer 115–241
    “ARCHITECTURE (user's suggestion, adopted): add native
     profiling flags to the rank tools…”
```

Each source appears once with its resolved locator, the markers it grounds, and
its verbatim excerpt.

## 5. Surface: Markdown (export, paste-survivable)

Plain CommonMark; no ANSI, no HTML. Must survive being pasted into an issue or
doc.

**Scan view — a table** (marker cell blank on continuation rows so a
multi-source marker reads as one group):

```markdown
#### Citations

| # | answer | p | source |
|--:|:--|:--|:--|
| 1 | 115–241 | 0.91 | codex:B149 |
|   |         | 0.87 | 7A59 |
|   |         | 0.71 | claude:E347 |
| 3 | 1244–1254 | 0.95 | codex:B149 |
```

**Audit view — blockquotes**, source-first, excerpt as the quote:

```markdown
**7A59** — Skill triage · src 965670–966914
grounds [1] (p=0.87, answer 115–241)

> ARCHITECTURE (user's suggestion, adopted): add native
> profiling flags to the rank tools…
```

## 6. Surface: HTML (rich, interactive)

The answer text **is** the citation surface. Each `reply_span` becomes a live
range in the rendered answer; the separate list becomes optional.

- **Inline spans.** Each cited answer range is underlined/tinted inline. Hover
  raises a card with that marker's sources — each with its own confidence pill +
  bar, resolved location, and excerpt. Multi-source markers list all sources.
- **Bidirectional highlight.** Hovering a span lights its entry in a reference
  rail and vice versa (the `reply_span ↔ [N]` link, made visible).
- **Display modes.** `underline` (per-strength color), `strength tint`
  (background heat = ambient grounding, underlines off), `clean` (invisible
  until hover). Same three modes offered in TUI reader mode.
- **Pin.** Click locks a card open to scroll long excerpts; Esc / outside-click
  dismisses.
- **Confidence** is per-source in the card, amber below threshold; the rail
  encodes each marker's source count and confidence band as dots.
- `[N]` stays a real superscript anchor so it survives even in clean mode.

**Open problem — overlapping spans.** Adjacent or nested `reply_spans` are real.
Options: split at boundaries into sub-spans, nest, or select the innermost on
hover. Unresolved; needs a decision before HTML ships.

## 7. TUI reader mode (proposed, tiered)

Porting the HTML interactions into a terminal. Hover becomes a keystroke or
mouse event; the floating card becomes a docked panel; pin becomes "always
docked." Three tiers by cost.

**Tier 1 — enrich the stream, no event loop.** OSC-8 links on `[N]` and source
handles, styled underlines on spans, per-source confidence with amber threshold.
Gated on the existing `isTerminal()`. ~80% of the value, no state machine. Ships
in the current printer. Requires capability detection (`TERM` / `TERM_PROGRAM`)
with layered fallback: link → bracketed `[N]`; curly underline → plain;
truecolor heat → 256-color → none.

**Tier 2 — reader mode (`nlm chat --read` or an inline `[v]` action).**
Alt-screen pager; needs a TUI runtime (e.g. bubbletea). Keyboard replaces hover:
`Tab`/`n` walks span-to-span, current span in reverse video, a **docked panel**
mirrors the HTML card (per-source rows + excerpts). Keys: `Tab`/`n` next, `p`
prev, `1`–`9` jump to marker, `⏎` open source, `e` expand excerpt, `/` filter by
source, `u`/`h`/`c` switch display mode, `q` exit to scrollback. This is where
the HTML experience lands for keyboard users.

**Tier 3 — mouse tracking.** Inside reader mode, if the terminal reports motion
(`DECSET 1003`), literal hover raises the panel with no keystroke; click pins;
scroll reads a long excerpt. A progressive enhancement of Tier 2's loop, not a
separate build. Do not gate the feature on the mouse.

### Interaction mapping

| Interaction | HTML | Terminal | Tier |
|---|---|---|---|
| See what's grounded | inline underline / tint | styled underline or bg-heat cells | 1 / 3 |
| Inspect a claim's sources | hovercard | docked panel follows selected span | 2 |
| Raise by pointing | `mouseenter` | mouse motion (DECSET 1003) | 3 |
| Raise without a mouse | — | `Tab`/`n` walk, reverse-video cursor | 2 |
| Pin / lock | click / checkbox | panel always docked; `space` pins on hover | 2 / 3 |
| Cross-highlight list ↔ span | peer class | selecting a span scrolls + inverts its list row | 2 |
| Follow a reference | anchor link | OSC-8 click, or `⏎` on selection | 1 / 2 |
| Per-source confidence | pill + bar | colored score + `▁▃▅▇` spark, amber < 0.75 | 1 |
| Switch display mode | segmented control | `u`/`h`/`c` underline·heat·clean | 2 / 3 |
| Read a long excerpt | scroll in card | `e` expand / mouse scroll | 2 / 3 |

## 8. Build order

1. **Renderer correctness (do first, independent of new surfaces).** Move
   confidence to the source row in both scan and audit views; label offsets
   `answer …` / `src …`; render `SourceStart/SourceEnd`. Delete the
   `uniformConfidence` header hoist. This fixes real information loss in the
   current output and is a small change. *(Done — commit `f1fcb3ea`.)*
2. **Self-contained persistence (§9).** Stamp `Title` (and confirm `Excerpt` /
   `SourceStart/SourceEnd`) onto every `Citation` at save time; make replay read
   from disk and treat the network as optional enrichment; warn instead of
   swallowing a replay-time auth failure. Do this before the new surfaces — each
   of them assumes the model is fully present offline.
3. **Markdown** scan-table + audit-blockquote renderers over the same model.
   *(Done — commit `107cd0f5`.)*
4. **HTML** static (spans + hovercards + rail), then resolve the overlap problem.
   *(Done — `107cd0f5`; adversarial-review fixes in `e4dbfb64`.)*
5. **TUI Tier 1** — OSC-8 links, styled underlines, capability detection. Ships
   in the streaming printer.
6. **TUI Tier 2** reader mode (bubbletea); **Tier 3** mouse as an enhancement of
   it.
7. **Rich content (§10)** — render the answer's block tree on the HTML/Markdown
   surfaces as progressive enhancement over the flat text. The prose tree and
   grounding path are verified lossless (`betool --verify`); do paragraphs /
   headings / lists / links first. Code blocks and tables are the only unverified
   corner — gate them on capturing one real code/table frame and confirming
   `--verify` is lossless. Independent of §9.

## 9. Persistence: a saved chat renders offline, with zero API calls

A stored chat should be **self-contained**: everything a renderer reads must
already be on disk, so replay (`nlm chat show`) does no network I/O, needs no
auth, and never degrades when a token expires. This is a data-model rule that
sits underneath every surface in this spec — a renderer can only project fields
that survived the save.

### Why this exists (observed failure)

`nlm chat show` currently prints citations with **only hex source IDs and no
names** whenever the stored session predates the excerpt work *or* was saved
without a successful title fetch. Traced to ground truth: replay resolves titles
by calling `GetProject` **live** (`notebookSourceTitles` → RPC `rLM1Ne`), and
when the local token is stale that call returns `API error 16 (Authentication):
Unauthenticated`. `notebookSourceTitles` **swallows the error** (`return ""`),
so `citationLabel` falls through to the handle-only branch. Excerpts, by
contrast, still render — because they were baked onto the `Citation` at an
earlier valid-auth save and read straight from disk. That asymmetry *is* the
tell: **what's persisted survives; what's re-fetched breaks.**

Confirmed on disk, not merely inferred from the code path: the HTML renderer's
JSON payload for a real replayed session carries `"title":""` for **all** of a
session's marker sources while `excerpt` is populated — the title field is empty
in the persisted data, not lost in rendering. Verified identically across all
three `--format` surfaces (text/markdown/html) on the same session.

### The rule

Persist, at generation/save time (when auth is valid by construction — you just
made the call that produced the answer), every field the renderers in §4–§7
consume:

| Field | Source at save | Persisted today? | Live-refetched on replay today |
|---|---|---|---|
| `SourceIndex`, `StartChar/EndChar` (answer span) | stream/history frame | yes | — |
| `SourceID`, `Confidence` | stream/history frame | yes | — |
| `Title` | `GetProject` (`rLM1Ne`) | **only if fetch succeeded at save** | **yes → breaks on stale auth** |
| `Excerpt`, `SourceStart/SourceEnd` | `GetConversationHistory` (`khqZz`) | yes on records saved post-excerpt-work | yes for older records |
| `file:line` (resolved locator) | `LoadSourceText` (`hizoJc`) | no | yes, under `--resolve-citations` |

`ChatMessage.Citations []api.Citation` serializes directly to the session JSON
(main.go), so filling these fields at save time is the *entire* mechanism — no
new store, no schema beyond the struct. `persistableCitations` already stamps
`Title` via `resolveTitle` at save; the gaps are (a) it stored empty silently
when the save-time fetch failed, and (b) older records predate it.

**The fix is renderer-free.** All three `--format` renderers already resolve a
source name as `resolved → c.Title → handle` (text degrades to
handle-or-quoted-title, markdown/html to the bare handle), so they already treat
the title as optional enrichment read from the persisted `Citation`. Once §9
stamps `Title` at save, replay renders names with **no change to any renderer**.
Scope is therefore just two edits: (a) make the save-time stamp in
`persistableCitations` robust, and (b) the single symmetric-warn below — one
site each.

### What still legitimately needs a live call

Only `--resolve-citations` file:line: source bodies are large and *mutable*
(a source can be re-uploaded, shifting offsets), so baking `file:line` into the
save would go stale. Persist the raw `SourceStart/SourceEnd` (already done) and
resolve to `file:line` on demand, cached. Everything else is immutable once the
answer is generated and belongs on disk.

### Consequences for the renderer

1. **Replay must not require a client.** The title/excerpt path should read the
   persisted `Citation` first and only reach for the network as an *optional
   enrichment* (e.g. `--resolve-citations`), never as the default source of a
   field that should have been saved. *(Already true of the three `--format`
   renderers as of `107cd0f5` — they read `c.Title` and degrade to the handle;
   the remaining live-fetch dependency is in `notebookSourceTitles`, not the
   renderers.)*
2. **Never swallow an auth error into an empty string.** When a replay-time
   fetch *is* attempted and fails, warn symmetrically with the excerpt path
   ("auth may be expired — run `nlm auth`; rendering names only"), so a
   recoverable auth lapse is diagnosable instead of silently producing nameless
   citations.
3. **Backfill is optional, not required.** Once titles/excerpts are persisted
   going forward, old records can be upgraded lazily: on a replay that *does*
   have valid auth, resolve and rewrite the session so the next replay is
   self-contained. Never a precondition for rendering.

## 10. Rich content: render the answer's structure, when the tree is trustworthy

The `GetConversationHistory` frame carries more than the flat answer text the
renderers use today. Alongside `content` (the plain string every surface splits
to runes) sits a **rich-text document tree** — decoded on branch `nlm-wt-betool`
as `RichDocument → SpanLayers → Span`. It models what the flat string discards:

- **Headings, paragraphs, list items with indent** (the `Span` block tree +
  `separator` boundaries + `SpanGroupMeta.indent` + `ListItem`) — verified present
  and lossless (see below).
- **Inline marks and links** (`TextMarks`: emphasis flags + a confirmed `link`
  target). Flag→semantic mapping (bold vs. italic vs. code) is not yet confirmed;
  `flag8` is observed on code/identifier runs.
- **Code blocks with a language** and **tables** (`SpanCodeBlock{code, language}`,
  `SpanTable`) — modeled by analogy but **not yet round-trip-verified** on a frame
  that contains them (see the sequencing table).
- A **per-source annotation layer** (`SourceAnnotation`) mapping rendered-document
  offset ranges back to their source — a third, layout-oriented source index,
  distinct from `grounding` and from `ContentAnnotation` (see §1's note on the
  two structures).

### Why this is required, not cosmetic: the flat text has no newlines

The server ships the answer **newline-free**. Decoding the real 173KB frame
(`internal/batchexecute/testdata/conversation_history.txt`): **13,043 string
values, zero contain a newline** — in the flat `content`, the excerpts, and the
rich-tree leaves alike. All document structure is carried by the **span tree**,
not by characters in the text: a paragraph break is a `separator` span (a
zero-width block boundary) plus the block `start/end` ranges, not a `\n`.

The consequence: the flat renderers **cannot** reconstruct paragraphs, lists, or
headings — there is nothing in the string to split on. Two adjacent paragraphs
render as one run-on line (`…harnessImplemented` / `notes2026-07-09` glued
together). Reading the rich tree is the *only* way to recover structure. In the
reference frame that structure is substantial: 3 `rich_document` turns totaling
~180 block spans (`content` ×131, `separator` ×15, `hidden_content` ×40 for
reasoning), 229 `listItem`s, and inline marks (`flag8` ×323 on code/identifier
runs, one `link`). This is not a formatting nicety layered on adequate text — the
flat text is structurally lossy by design, and the tree is where the structure
went.

### Two structures, don't confuse them

The wire splits "what a marker points at" from "what grounds it":

- **`grounding` (`GroundingRecord`/`Grounding`)** is the *evidence*: per source,
  it carries `score`, `reply_spans`, `source_spans` (the **excerpt** +
  `SourceStart/End`), and the source UUID. **This is what `Citation` is built
  from.** Excerpt fidelity is already complete on this path.
- **`annotation` (`ContentAnnotation`, `SourceAnnotation`)** is the *index*: a
  reply offset range → source indices (or a rendered-doc range → source id). No
  score, no excerpt. Thin pointer, one dereference from the grounding.

So the rich tree does **not** improve citation *evidence* (excerpts already come
through flat). It improves the *answer body's* rendering — real code blocks,
tables, and lists instead of an undifferentiated paragraph — and offers an
alternate citation *layout* layer we do not need.

### Should we render from it? Yes for the body — the decoder is now verified.

The gain is real: today a Go code block in an answer renders as flat prose in
HTML and Markdown. The rich tree would let those surfaces render the answer the
way NotebookLM formatted it.

**What is now verified (2026-07-21, branch `nlm-wt-betool`).** The
`GetConversationHistory` proto re-encodes **losslessly** — `nlm betool
decode-response --proto --verify` on a real 173KB frame
(`internal/batchexecute/testdata/conversation_history.txt`) reports
`verify: lossless`. That frame exercises the citation/grounding path heavily
(58 grounding records, 108 `source_spans`) and carries the rich-document tree
(3 `rich_document` turns). So the **container, the block/paragraph/heading/list
spans, and the whole grounding path are proven complete on real data** — not
merely shape-matched. Newly modeled since this section was first written:
`TextMarks.link` (hyperlink targets) and the `SpanElement` scalar-or-span union.

**What is still inference, not round-trip-verified.** The lossless frame contains
**no code blocks and no tables** (`code_block`/`SpanTable`/`language` count: zero).
`SpanCodeBlock` and `SpanTable` are modeled "by analogy / not yet confirmed" from
captures observed at authoring time but **not preserved as a verified fixture**.
Likewise `RichDocument.flag` ("meaning TBD") and the `TextMarks` flag→semantic
mapping (which flag is bold vs. code — "not yet confirmed"). These are exactly the
highest-value pieces, so the gate is concrete, not vague: **before rendering from
`SpanCodeBlock`/`SpanTable`, capture one real frame that contains a code block or
table and confirm `betool --verify` reports lossless on it.** That is a small,
checkable step — not "wait for the whole proto to be perfect."

Coupling render quality to an *un-verified* corner of the decoder is a fragility
trade: a wire reshape there becomes a rendering regression. The verified corners
(prose tree, grounding) do not carry that risk. And the flat `content` +
offset-span model must stay regardless — it backs `--citations=json`, the TUI, and
offline replay — so the rich path is a *second* answer model the renderers must
project without diverging.

### Sequencing (by what is round-trip-verified, not all-or-nothing)

| Piece | Decoder state | Render value | Verdict |
|---|---|---|---|
| Answer **paragraphs / headings / lists** | **verified lossless** on real frame | high | do first — HTML/MD only |
| Answer **links** (`TextMarks.link`) | modeled, present in tree | medium | with the block tree |
| Answer **code blocks / tables** (`SpanCodeBlock`/`SpanTable`) | inferred, **no verified fixture** | high | gate: capture + `--verify` a code/table frame first |
| Answer **bold / mark semantics** (`TextMarks` flags) | flag→meaning unconfirmed | low | render generically or defer |
| Citation **`SourceAnnotation` layout layer** | modeled | marginal (excerpts already flat) | defer indefinitely |

Rules for whoever picks this up:

1. **Progressive enhancement only.** Render the rich tree when it is present *and*
   decodes cleanly; fall back to the flat `content` path otherwise — always, per
   surface. The TUI and `--citations=json` stay flat. This mirrors §9: never let a
   fragile decoder be the *only* source of something the reader needs (here, the
   answer showing up at all).
2. **Gate per block type, not per frame.** An unknown or unmodeled block type
   degrades to its flat text slice (the `Span` still bounds `start/end` into the
   document), not to a failed render.
3. **Keep the "one model" seam.** The rich tree becomes an *optional* field on the
   `chatDocument` message alongside flat `content`; renderers project from it when
   set. It does not replace the flat model or fork the citation model.
4. **Not blocked on §9.** Independent work; excerpts and titles land first.

### Offset alignment: the one thing that silently breaks (verified)

Citation `[N]` hovercards key off `reply_span` char offsets; the rich spans key
off their own `start/end`. If those index different spaces, a citation lands on
the wrong text — worse than run-together prose. Checked against the real frame:
they share **one** offset space (on an assistant turn, the rich block spans and
the citation annotation ranges both max out at the same end offset, 959, with
ranges interleaved at the same scale). **So align by wire offset, never by a
recomputed flattened length** — do not assume `len(flatten(tree)) ==
len(content)` and slice by it; index both the citation spans and the rich spans
by the offsets the wire gives.

Three properties of those offsets, each of which desyncs rendering if unhandled
(all observed in the reference frame):

- **Blocks are not in document order.** The block list arrives unsorted
  (`(0,115) (1143,1228) (115,432) …`); **sort by `start`** to reconstruct reading
  order.
- **Offsets decode as strings** (`"0"`, `"115"`), not ints — a beprotojson
  int64-as-string artifact. Parse to int before any comparison or slice; the
  parse layer (b) should surface them as `int64` and assert it in a fixture.
- **Flattening must recurse.** Top-level `body.blocks` can cover only part of the
  document (one turn's top blocks reached offset 77 while content ran to 876); the
  remainder lives in nested `SpanGroup` children. A `body.blocks`-only walk
  undercounts and desyncs — recurse through `SpanContent.group`.

The alignment invariant to assert (not length equality): every citation
`reply_span [s,e]` maps onto a contiguous run of recursively-flattened rich leaf
spans whose ranges **cover** `[s,e]` — offset containment. The parse layer should
emit a fixture with the parsed tree *and* the citation spans from the same frame
so a renderer test can assert containment before hovercards are wired onto
tree-rendered output.

### Porting order and current state (2026-07-21)

Three layers, and the middle one is the sole remaining blocker:

1. **(a) Wire layer** — `RichDocument`/`SpanLayers`/`Span`/`SpanContent`/`TextLeaf`/
   `TextMarks` proto + regenerated `pb.go` + beprotojson shape-union selector.
   **DONE — committed on `nlm-wt-betool`** (proto + `gen/…pb.go`; a real frame
   re-encodes `verify: lossless`). *(proto/betool turf.)*
2. **(b) Parse layer** — `parseConversationHistory` decodes the segment's rich
   payload into a parsed `RichDocument` and hangs it on `ChatMessage` as an
   *optional* field beside `Content` (rule 3 — add, don't fork); plus the
   alignment fixture below. **NOT STARTED — the blocker.** *(api turf.)*
3. **(c) Render layer** — `chatShow` threads the parsed tree onto `chatDocument`;
   the three renderers project it, gated on presence (rule 1). **DONE (scaffold) —
   committed on `work/citation-excerpts`** against a branch-local stub
   `RichDocument` (with the gotchas above baked into its tests: unsorted blocks,
   string offsets, nested group). Threaded-ready. *(renderer turf.)*

**Why (b) waits for a merge, not a cherry-pick.** The rich decode is not just
proto+gen — it is coupled to `beprotojson`'s `shapeUnion` machinery, which evolved
across ~112 commits on `nlm-wt-betool` (including a recursive-span-verification
fix) that is not on `main`. Cherry-picking a minimal proto slice onto
`work/citation-excerpts` would leave the decoder inconsistent. So (b) should be
based on a branch where proto + gen + the matching `beprotojson` are all
consistent — i.e. after `nlm-wt-betool` lands on `main` (or a shared base) — then
the small (c) scaffold rebases onto it. That is the easy merge direction (rebase
2 render commits onto a stable base, not drag 112 decoder commits onto the render
branch).

**The (b) contract** (so it is unambiguous when it starts):
- Add an optional `RichDocument` field to `api.ChatMessage` beside `Content`;
  populate it from the segment's rich payload, leave it nil when absent. Offsets
  surface as `int64` (proto declares `int64`; beprotojson's JSON form is
  int64-as-string — the Go decode gives real ints).
- Ship a **fixture from one real frame** carrying, together: flat `Content`, the
  parsed `RichDocument`, and the citation `reply_spans`. Fabricated-text /
  real-structure (offsets and tree shape preserved, words replaced) per the
  privacy rule — offsets survive fabrication and are all the containment test
  needs.
- The fixture must include **at least one citation whose `reply_span` sits after a
  hidden block**, to settle the visible-vs-inclusive offset-space question. Early
  evidence from the synthetic frame: top-level `hidden_content` blocks carry **no
  offsets** (0 of 11 had a `start`), suggesting hidden ranges are *outside* the
  visible coordinate space citations index — i.e. the "exclude hidden" branch of
  the render scaffold's `TestHiddenBlockOffsetGap`. Confirm on the real fixture
  before deleting the alternative branch.

## 11. Open questions

- **Resolved locator vs. raw offset.** In the audit view, is `file:line` (via
  `--resolve-citations`) the primary anchor with raw `src N–M` as fallback, or
  are both always shown?
- **Compact per-source scores.** Does the scan view show every source's score,
  or collapse to a range (`4 sources, p 0.68–0.91`) and expand only under
  `--citation-excerpts`?
- **Reader-mode entry.** Separate `--read` invocation, or an inline `[v]` action
  after a streamed answer (one flow: read → `v` → inspect → `q`)? The inline form
  is more discoverable but the printer must hand its citation model to the pager
  without a re-fetch.
- **Overlapping HTML spans.** Split, nest, or innermost-on-hover.
- **`[N]` as label vs. footnote anchor.** Bracketed everywhere, or real footnote
  anchors in HTML/Markdown.

## References

- Wire model verified with `nlm betool decode-response --proto` (proto encoder
  lives on branch `nlm-wt-betool`; not a dependency of the citation code).
- Data model / parser: `notebooklm/client.go` (`Citation`,
  `parseCitationsV2`, `parseConversationHistory`).
- Current renderer: `cmd/nlm/main.go` (`renderCitationList`,
  `renderCompactGroup`, `renderExpandedGroup`, `citationGroupHeader`).
