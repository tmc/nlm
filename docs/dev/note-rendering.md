---
title: Note Rendering Design
date: 2026-07-24
---

# Note Rendering Design

How `nlm` should turn a NotebookLM **note** into a rich, self-contained HTML
artifact that works offline. This is a sibling to
[Citation Rendering](citation-rendering.md), but the two share almost no
machinery: a chat answer arrives with a decoded span tree and attached
grounding, whereas a note is **markdown text with nothing attached**. This doc
records that difference, so the note path is not mistakenly built on the chat
path's tree/offset/grounding apparatus.

## Status

- **Proposed, not built.** This doc is the design; no note-rendering code exists
  yet (`note read`/`note list` print raw text only, `cmd/nlm/main.go`
  `readNote`/`listNotes`).
- The **note wire shape** is verified against a real note — see §1.
- The **chat** rich-render path (span tree at `inner[0][4]`, UTF-16 offset
  mapping, parent-source hop, per-source excerpts) is shipped on
  `work/citation-excerpts` and is **out of scope here** — it does not apply to
  notes (§2).

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

A note is fetched via `GetNotes` (`internal/notebooklm/api/client.go`) and
carried as `pb.Note`. The two body-bearing fields:

- `ContentText` — wire field 1. Flat body text.
- `RichText` — wire field 5. In every note observed this is **also markdown**
  (or empty), **not** a structured tree.

`readNote` prefers `RichText`, falling back to `ContentText`. Either way what
surfaces is **markdown with real newlines** — `#`/`###` headings, `**bold**`,
`*italic*`, `` `code` ``, `---`, ordered/unordered lists — verified on the
SiPhon note (29 markdown-heading lines, 0 HTML tags) and on unrelated reachable
notes (Cybertruck, ModelIR, macOS notebooks). There is no span tree layer and no
per-passage grounding structure anywhere in the note payload.

**Consequence:** the note's own markdown *is* the structure. Rebuilding
paragraphs/lists/headings means parsing that markdown, not walking a tree.

## 2. Why the chat path does not apply

The [Citation Rendering](citation-rendering.md) work renders a chat answer from
a span tree delivered live at `inner[0][4]`, maps UTF-16 wire offsets to runes
before slicing, and hangs per-source hovercards (title, confidence, excerpt,
resolved via the §9 parent-source hop) off `[N]` markers whose grounding arrives
in the same frame.

None of that is present for a note:

| | Chat answer | Note |
|---|---|---|
| Structure source | span tree (`inner[0][4]`) | markdown text only |
| Offsets | UTF-16 wire offsets → runes | n/a (no offset payload) |
| `[N]` markers | backed by live grounding (source id, score, excerpt) | **detached glyphs** — no grounding in the note |
| Title resolution | §9 parent-source hop | n/a |

So the note renderer is a **markdown → HTML** problem, not a tree-projection
problem. Reusing the chat renderer's offset/grounding code for notes would be
building on structure that isn't there.

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
