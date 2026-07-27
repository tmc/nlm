---
title: Note Rendering Design
date: 2026-07-24
---

# Note Rendering Design

How `nlm` should turn a NotebookLM **note** into a rich, self-contained HTML
artifact that works offline. It is a sibling to
[Citation Rendering](citation-rendering.md): a rich note carries the **same
span-tree document and grounding** a rendered chat answer does, so the note path
should reuse the chat renderers rather than reinvent them.

## Status

- ★★ **SHIPPED (2026-07-26).** Both halves are done on `stage/integration`:
  `5a17d902` stops the flatten (`api.Note` now carries `Rich *pb.RichDocument` +
  `Grounding`), and `8cd9b152` renders it — `note read --format
  text|markdown|html [--out] [--open]` projects the tree through the shared chat
  renderer, resolves `[N]` from the note's own grounding, and falls back to a
  markdown subset for plain-arm notes. Verified live on the motivating note
  (106-block document arm → 187 KB structural, self-contained, XSS-safe HTML).
  Not pushed. §3–6 below are the as-built design; the flatten described in §1 is
  the *former* behavior, now fixed.
- ★ **PREMISE CORRECTED (2026-07-24).** An earlier draft of this doc claimed a
  note is "markdown text with nothing attached" and that no note rich modeling
  existed. Both are now **false**. The rich-note wire — a `RichDocument` span
  tree plus per-source grounding — is fully modeled and decoded (proto commits
  `74dfa8ca` "model rich note documents" and `5cd79bc4` "model rich-note payload
  on CreateNote"): `NoteRichText` (a `oneof {plain_text | RichDocument document}`
  union), `GetNotesRichRecord`, `GetNotesRichWireResponse`, `CreateNoteRichRecord`,
  and `NoteGroundingDetails` all exist in `orchestration.proto`.
- **The remaining gap is render-side, not proto-side.** `GetNotes` decodes the
  union then **flattens it** — `noteFromRecord` (`client.go`) takes
  `note.GetRichText().GetPlainText()` and drops the `document` arm + grounding
  into the flat public `pb.Note.rich_text` string; `readNote`/`listNotes` print
  that string. The tree and grounding are decoded losslessly and then thrown
  away at the API boundary. Nothing renders them.
- The **chat** rich-render path (span-tree decode, UTF-16 offset mapping,
  per-source grounding/excerpts, HTML/markdown tree renderers) is shipped and is
  the machinery to reuse — a rich note is the *same* `RichDocument`, so §3's
  renderer is largely already written for chat.

## Motivating example

The design is grounded in one real note (ids and title elided; it is a
private notebook). It is a promoted deep-panel answer: ~26 KB of markdown,
166 lines. Census:

- 29 ATX headings (`#`–`#####`), 65 `**bold**`, 51 `` `inline code` ``,
  46 bullet lines, 9 numbered items, 8 `---` rules.
- **67 LaTeX math expressions** (65 inline `$…$`, 2 display `$$…$$`).
- **45 citation markers** up to `[98]`, in three syntaxes: single `[1]`, list
  `[13, 14]`, range `[5-7]`.
- 0 HTML tags, 0 tables, 0 blockquotes, no References/Sources section.

The owning notebook has 48 labeled sources and one server-side conversation
(`4e4d504e`). Both matter to §4.

## 1. The model (ground truth)

A note is fetched via `GetNotes` (cFji9) and, on the wire, its rich body is a
shape union — **`NoteRichText`** (`orchestration.proto`):

```
message NoteRichText { oneof value {
    string plain_text = 1;
    RichDocument document = 2;   // same tree as rendered chat content
}}
```

Older/plain notes take the `plain_text` arm; **rich notes carry a full
`RichDocument` span tree** (headings, paragraphs, lists, tables, code blocks —
the identical structure the chat answer path renders). Alongside it,
`NoteGroundingDetails grounding_details` (field 4) carries per-source grounding
whose items are byte-for-byte equal to the `Grounding` part of the chat
`RichDocument.grounding`. So a rich note carries **both** the structure and the
grounding for its `[N]` markers.

The response record is `GetNotesRichRecord` (in `GetNotesRichWireResponse`);
`CreateNoteRichRecord` is the CYK0Xb equivalent. Both are decoded losslessly
today.

**The catch:** the public `pb.Note` is still the flat one (`string rich_text`),
and `noteFromRecord` (`client.go`) collapses the union with
`GetRichText().GetPlainText()` — taking the string arm and **discarding the
`document` tree and the grounding**. `readNote`/`listNotes` then print that flat
string. The markdown a plain `note read` shows is that flattened projection, not
evidence the tree is absent — the tree is decoded and then dropped.

## 2. Reuse the chat path — the note IS the same tree

Because a rich note's body is the same `RichDocument` the chat renderer already
projects to HTML/markdown, the note path should **reuse the chat machinery**, not
reinvent a markdown parser:

| | Chat answer | Rich note |
|---|---|---|
| Structure source | `RichDocument` span tree | **same `RichDocument`** (`NoteRichText.document`) |
| Offsets | UTF-16 wire offsets → runes | same |
| `[N]` grounding | per-source `GroundingRecord` in-frame | **`NoteGroundingDetails`, in the note** — resolvable without a source-conversation refetch |

The one place notes differ from chat: a **plain** note (union arm 1) has only
markdown text and no tree — that arm still needs a markdown→HTML fallback (§3).
So the renderer is: rich arm → project the tree via the chat renderer; plain arm
→ the markdown-subset fallback.

## 3. Structure rendering (the core deliverable, fully self-contained)

This is where nearly all the value is and it needs nothing external.

- **Input:** the note's markdown from `note read`.
- **Renderer:** a small Go markdown-subset renderer feeding `html/template`,
  reusing the **fixed-tag, auto-escaped, XSS-hard** pattern already proven in
  `cmd/nlm/chat_render_html_answer.go`. The note vocabulary is narrow and
  closed: ATX headings (`#`–`#####`), ordered/unordered lists (with nesting),
  `**bold**` / `*italic*` / `` `code` ``, `---` rules, paragraphs. A focused
  subset renderer, not a general CommonMark engine.
- **No new dependency.** `blackfriday/v2` is in the module graph but `//
  indirect`; using it would promote it to a direct dep (a `go.mod` change) and
  cuts against the minimal-deps house style. A closed-vocabulary renderer is
  also a *stronger* CSP boundary for the artifact than a general HTML-emitting
  library — the same reason the chat HTML path uses fixed-tag templates over
  dynamic `{{.Tag}}` (a dynamic tag name defeats `html/template` contextual
  escaping).
- **XSS hard requirement:** keep the chat path's guarantee — hostile note text
  (`</script>`, `<img onerror>`, `]]></template>`) must render inert. Fixed-tag
  templates + `html/template` auto-escaping hold this; verify with an
  `x/net/html` round-trip as the chat renderer does.
- **Self-contained:** inline CSS/JS only, zero external requests, works
  `file://` and under the artifact CSP.

### LaTeX math — the one genuinely new element

67 expressions on the SiPhon note; earlier note evidence had none, so this is
note-specific. The artifact CSP **blocks external hosts**, which rules out
loading KaTeX/MathJax from a CDN. Tiers, cheapest → richest:

1. **Verbatim monospace** (recommended first cut): render `$…$` / `$$…$$` in a
   `code`-styled span/block. Honest, zero-dep, offline-safe, never wrong.
2. **Light typographic massaging** (superscripts, Greek letters): brittle,
   easily wrong on real algebra — not recommended.
3. **Vendored renderer:** inline a full KaTeX JS + font bundle as data-URIs to
   satisfy the CSP. Publication-grade, but a large self-contained payload and a
   real maintenance surface. Only if publication-quality math is a stated goal.

Start at tier 1; revisit only on an explicit ask.

## 4. Citation markers (`[N]`) — deferred, but feasible for this note

The note's `[N]` glyphs carry no grounding *in the note*. Unlike the stranded
cases in the citation-rendering history (sources re-synced away → dead ids),
this note's 48 sources still exist and its source conversation `4e4d504e` is
live server-side, so re-attachment is *possible* — but non-trivial:

- `[N]` are **answer-relative indices from the original panel run**. There is no
  guarantee `[50]` is the 50th entry of `source list`. The only reliable mapping
  is the source conversation's grounding frame.
- Recovering it means `GetConversationHistory` on `4e4d504e` (server-side; note
  `chat show` took the *local-session* path and missed it — the server route is
  needed), then aligning the promoted note text back to the answer turn (the
  §9 `citationContentKey` content-signature approach) to graft each turn's
  grounding onto the note's markers.
- This is speculative until proven on this data and adds real complexity.

**Decision: defer.** Render `[N]` as styled-but-inert superscripts in the first
cut; treat live re-attachment as an optional Phase 2. The markdown artifact
stands on its own without it.

## 5. Surface and plumbing

- Expose as `note read --format html [--out FILE] [--open]`, parallel to
  `chat show --format {text,markdown,html}`. `--out`/`--open` are html-only, as
  in `validateChatFormat`.
- **Do not force a note through `chatDocument`.** That model is chat-shaped
  (turns, per-source grounding, offset spans). A note is simpler; a thin
  `noteDocument` sibling — title + parsed markdown blocks — is cleaner than
  bending the chat model around a payload that lacks its central features.
- Reuse from the chat path: the `html/template` fixed-tag seam, the
  self-contained-artifact CSS/JS conventions, and the `x/net/html` XSS
  round-trip test discipline.

## 6. Plan

1. `noteDocument` model + markdown-subset parser (headings/lists/emphasis/
   code/rules/paragraphs), table-driven tests on the closed vocabulary.
2. `html/template` fixed-tag renderer → self-contained HTML; math verbatim
   (tier 1); `[N]` inert superscripts; XSS round-trip test.
3. Wire `note read --format html [--out] [--open]` + flag validation.
4. (Optional Phase 2) fetch `4e4d504e` server history, align by content-key,
   graft real grounding onto `[N]`.

## Open questions

- **Phase 2 scope:** re-attach citations, or leave `[N]` inert? (§4)
- **Math tier:** verbatim monospace vs. vendored KaTeX. (§3)
- **Source of truth on the wire:** confirm whether any note ever populates
  `RichText` (field 5) with something *other* than markdown before assuming
  markdown-only universally. All observed notes are markdown, but the sample is
  small.
