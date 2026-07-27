---
title: Parity Matrix — nlm vs the NotebookLM CLI/MCP ecosystem
---
# Parity Matrix

Status: evidence · 2026-07-27 · Method: [parity-notebook.md](parity-notebook.md)

Grounded competitive analysis produced by syncing seven tools' source into a
NotebookLM notebook and running one grounded, citation-backed chat per dimension,
then **verifying every load-bearing claim against the cloned trees** (the model
hallucinates paths; unverified rows were dropped or corrected). Notebook:
`<notebook-id>`.

## The field

| Tool | Lang | Kind | Repo |
|---|---|---|---|
| **tmc-nlm-go** (this) | Go | CLI + MCP | github.com/tmc/nlm |
| notebooklm-mcp-cli-py | Python | CLI + MCP | jacob-bd/notebooklm-mcp-cli — **the name-collision `nlm`** |
| notebooklm-cli-py | Python | CLI | jacob-bd/notebooklm-cli (legacy) |
| notebooklm-cli-go-dokkabei | Go | CLI | Dokkabei97/notebooklm-cli — **also pitches "single static binary"** |
| notebooklm-mcp-m4ykel | TypeScript | MCP | m4yk3ldev/notebooklm-mcp (32 tools) |
| notebooklm-mcp-pleaseprompto | TypeScript | MCP | PleasePrompto/notebooklm-mcp (v2, DOM automation) |
| notebooklm-mcp-alfredang | Python | MCP | alfredang/notebooklm-mcp (delegates to notebooklm-py) |

## Dimension matrix

Legend: ✅ present · ➖ partial/limited · ❌ absent (verified "not found in sources").

| Dimension | tmc-nlm-go | jacob mcp-cli | jacob cli (legacy) | dokkabei (Go) | m4ykel (TS) | pleaseprompto (TS) | alfredang (Py) |
|---|---|---|---|---|---|---|---|
| **Direct RPC (not DOM)** | ✅ | ✅ | ✅ | ✅ | ✅ | ❌ DOM | ➖ delegated |
| **Compiled protobuf wire model** | ✅ **only one** | ❌ positional-JSON | ❌ | ❌ positional-JSON | ❌ positional-JSON | ❌ | ❌ |
| **Lossless-verify tool (betool)** | ✅ **only one** | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ |
| **Headless profile auth** | ✅ | ✅ | ✅ | ✅ (Keychain) | ✅ | ✅ | ➖ |
| **Silent token recovery** | ➖ reactive profile re-harvest | ✅ 3-layer | ✅ 3-layer | ❌ | ✅ single-flight | ✅ (browser ctx) | ❌ |
| **Multi-account** | ➖ authuser | ✅ profiles | ✅ profiles | ❌ | ❌ | ✅ --account | ❌ |
| **Interactive chat REPL** | ✅ | ✅ | ✅ | ❌ | ❌ (MCP only) | ❌ | ❌ |
| **On-disk session history** | ✅ | ❌ (server-side) | ❌ | ❌ | — | — | — |
| **Source select by name/regex** | ✅ **only one** | ❌ IDs only | ❌ IDs only | ❌ IDs only | ❌ | ❌ | ❌ |
| **One-shot scriptable chat** | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| **Bundle many local files → 1 source** | ✅ (txtar sync) | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ |
| **Local file upload** | ✅ | ✅ | ❌ | ✅ | ✅ | ✅ | ➖ |
| **Deep research → clean Markdown** | ✅ + URL footnotes | ✅ | ✅ | ✅ | ✅ | ❌ | ➖ |
| **Artifact export to local file** | ➖ decks/audio/video | ✅ **broad** | ❌ | ➖ generic | ❌ | ➖ audio only | ❌ |
| **Flashcard/quiz export** | ❌ | ✅ json/md/html | ❌ | ➖ | ❌ | ❌ | ❌ |
| **Mindmap export** | ❌ | ✅ JSON | ❌ | ❌ | ❌ (removed) | ❌ | ❌ |
| **Ships MCP server** | ✅ | ✅ | ❌ | ❌ | ✅ | ✅ | ✅ |
| **MCP context injection** | ✅ note+text | ✅ | — | — | ✅ | ➖ | ➖ text |
| **MCP watch/progress notify** | ❌ (poll) | ➖ async status | — | — | ❌ | ❌ | ❌ |
| **Single static binary** | ✅ | ❌ Python | ❌ Python | ✅ | ❌ Node | ❌ Node | ❌ Python |

## Broader feature coverage (second sweep, verified)

Legend: ✅ · ➖ partial · ❌.

| Dimension | tmc-nlm-go | jacob mcp-cli | jacob cli | dokkabei | m4ykel | pleaseprompto | alfredang |
|---|---|---|---|---|---|---|---|
| **Note CRUD** | ✅ | ✅ | ❌ | ✅ | ❌ | ❌ | ➖ (mindmap only) |
| **Note rich-tree render (md/html)** | ✅ **only one** | ❌ plain text | ❌ | ❌ plain text | ❌ | ❌ | ❌ |
| Save chat answer → note | ➖ | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ |
| Inline citations in output | ✅ | ✅ | ✅ | ➖ raw | ✅ | ✅ | ✅ |
| **Citation → source excerpt text** | ✅ | ✅ | ❌ | ❌ | ❌ | ✅ (DOM) | ❌ |
| **Citation → file:line (txtar)** | ✅ **only one** | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ |
| Rich citation rendering (html/footnotes) | ✅ **most complete** | ➖ legend | ➖ legend | ❌ | ❌ | ✅ 3 formats | ❌ |
| **Server-side labels (NLM label RPCs)** | ✅ full suite | ✅ + AI auto-label | ❌ | ❌ | ❌ | ❌ | ❌ |
| Local notebook tags/select | ❌ | ✅ smart-select | ❌ | ❌ | ❌ | ✅ local lib | ❌ |
| Collections | ❌ | ❌ (schema only) | ❌ | ❌ | ❌ | ❌ | ❌ |
| **Directory-tree sync** | ✅ **only one** | ❌ (Drive only) | ❌ | ❌ | ❌ | ❌ | ❌ |
| Incremental/idempotent re-sync (sha256) | ✅ **only one** | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ |
| Chunking large content | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ |
| `.nlmignore` / exclude globs | ✅ | ➖ allowlist | ❌ | ❌ | ➖ sandbox | ❌ | ❌ |

Notes from this sweep:
- **Note rich rendering is uniquely ours.** Every other tool that has notes treats
  them as plain-text strings; only `tmc-nlm-go` renders the RichDocument tree
  (headings/lists/tables/bold) to Markdown/HTML (`internal/richrender`,
  `projectRichDocument`). Same for the citation *rendering* suite (interactive
  HTML with hovercards + a reference rail — `cmd/nlm/chat_render_html.go`) which
  is the most complete in the field; pleaseprompto is second (3 text formats).
- **Citation → file:line for txtar sources is uniquely ours**
  (`cmd/nlm/resolve_citations.go`). It pairs with the directory-sync feature: no
  other tool bundles a tree *or* resolves citations back into it.
- **Directory sync is uniquely ours and genuinely deep** — verified sha256
  content-cache for idempotent re-sync (`internal/nlmsync/cache.go`), auto-chunking
  at `--max-bytes`, and `.nlmignore` support (`internal/nlmsync/files.go`). No
  competitor has any local-tree sync at all (jacob-bd's "sync" is Drive-only).
- **Labels: a wash, with a twist.** We have the full *server-side* NLM label RPC
  suite (list/create/rename/emoji/attach/relabel-all). jacob-bd matches that and
  adds **AI auto-labeling** + *local* notebook tagging / smart-select (verified:
  `services/labels.py:auto_label`, `smart_select.py`). Neither exposes
  Collections (the proto defines them; nobody wires them). AI auto-label is a
  real feature we lack.
- **Collections: nobody has them.** Confirmed open across the whole field.

## Where nlm is *uniquely* ahead (verified)

1. **Wire-protocol depth — decisive.** `tmc-nlm-go` is the **only** tool that
   compiles NotebookLM's positional-array wire format into protobuf types
   (`gen/notebooklm/v1alpha1/`, `internal/beprotojson`) and ships a
   lossless-verification inspector (`betool`, `cmd/nlm/betool.go`) that decodes
   raw HTTP payloads to typed protos and diffs for round-trip fidelity. Every
   other RPC-based tool uses hardcoded positional indices (`params := []any{...}`,
   `arr[0]`); one (pleaseprompto) doesn't touch the protocol at all — it drives a
   stealth Chromium DOM. This is a real, defensible moat and the core of the
   positioning story.
2. **Source selection by name/label/regex** — every other tool takes source
   UUIDs only (`--source-ids`, `-s`). We match on title/label/regex
   (`cmd/nlm/source_match.go`).
3. **Bundling many local files into one source** via txtar sync — no other tool
   does this.

## Where nlm is genuinely behind (verified — the real tail)

1. **Broad artifact export (biggest gap).** jacob-bd's `notebooklm-mcp-cli` ships a
   broad download suite (`services/downloads.py`): quiz/flashcards → json/md/html,
   slides → pdf/pptx, mindmap → JSON, report → Markdown, data-table → CSV. We
   download rendered slide decks as PDF/PPTX and have audio/video download paths,
   but no general export command or flashcard/quiz/mind-map/report/data-table
   emitters. This is spec item **B1**, now confirmed as a place multiple
   competitors lead.
   - *Nuance:* their export pulls **server-rendered** outputs; nobody (including
     them) extracts raw flashcard front/back/deck content. So an Anki `.apkg` /
     structured-card emitter is still an open niche — but the *basic* study-export
     use case is closed for them and open for us.
2. **Proactive token refresh.** Both jacob-bd tools have a 3-layer refresh
   (`core/auth_refresh.py`, verified present); m4ykel has single-flight refresh.
   Ours instead re-harvests the exact cached browser profile after an
   authentication-shaped failure and retries once. That closes interactive
   recovery, but environment-only CI credentials cannot self-renew.
3. **Multi-account.** Competitors use named profiles / `--account`; we have only
   `--authuser N`. Partial parity at best.
4. **AI auto-labeling.** jacob-bd's `services/labels.py:auto_label` clusters a
   notebook's sources into labels via AI (verified). We have the full label RPC
   suite but no auto-labeler.
5. **MCP breadth + agent onboarding.** Their MCP server has 43 tools (vs our 38),
   HTTP/SSE transport, in-MCP `refresh_auth`, an `nlm --ai` docs flag, and a
   skill installer. See [parity-mcp.md](parity-mcp.md) for the full MCP + skills
   comparison and a prioritized roadmap.
6. **MCP watch/progress.** jacob-bd exposes async status tools; we poll. Minor
   (spec B5).

## At parity (no action needed)

Headless profile auth, interactive REPL, one-shot scriptable chat, deep-research
Markdown, local file upload, MCP injection — all present in `tmc-nlm-go` and
match or exceed the field. **The external reviewer's claim that these are
missing is false** — verified against our own synced source.

## Positioning consequence

- The "single static binary" line is **not unique** — Dokkabei97's Go tool makes
  the identical pitch (README: *"nlm is a single static binary… ideal for
  CI/CD"*, `CGO_ENABLED=0`). Do not lead on it alone.
- Lead instead on the combination only we have: **deepest wire modeling
  (protobuf + lossless betool) + CLI + interactive + MCP in one static binary +
  name/regex source selection**. That stack is unmatched in the field.
- Close **B1 (artifact export)** and **B2 (refresh)** to erase the two real
  deficits; then the matrix is ahead-or-even on every row.

## Provenance

- Notebook `<notebook-id>`, 7 sources, synced 2026-07-27
  from `git clone --depth 1` (manifest: a local manifest).
- Raw per-dimension grounded transcripts: a local working directory.
- Every ✅/❌ in the "ahead"/"behind" sections was re-checked against the cloned
  tree, not taken from the model on faith. Rows the model asserted but a grep
  couldn't confirm were downgraded or dropped.
