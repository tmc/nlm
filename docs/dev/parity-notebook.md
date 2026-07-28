---
title: Parity Notebook — self-hosted competitive analysis
---
# Parity Notebook: dogfooding nlm to assess its own standing

Status: completed evidence run · 2026-07-27

## Purpose

Use `nlm` itself to answer one question with evidence, not opinion: **is `nlm`
at parity with, behind, or ahead of the other NotebookLM CLI/MCP tools in the
ecosystem?** We build a NotebookLM notebook whose sources are the *actual source
trees and blog posts* of every comparable tool, then chat against it to produce
a grounded, citation-backed feature matrix.

This is dogfooding with a payoff: it stress-tests `nlm sync` on real
multi-repo input, and it yields grounded competitive-analysis input for the
project's positioning and launch material.

## Why this is the right tool for the job

NotebookLM is a grounded-RAG surface: every answer cites the source span it came
from. A feature-parity claim ("tool X has headless auth, we don't") is exactly
the kind of assertion that must be citation-backed, because the external
reviewer that started this whole thread got it *wrong* by reading repo surfaces
without grounding. Putting the competitors' real code into a notebook and asking
a grounded model to compare is the antidote to that failure mode.

## Ingredients

### The corpus (one source per tool)

Each tool becomes one txtar source built by `git clone` + `nlm sync`. Candidate
set (verify each still exists / is the right repo before syncing — do not sync a
URL that 404s):

| Tool | Language | Notes |
|---|---|---|
| `tmc/nlm` (this repo) | Go | the subject; sync it too so the panel can cite *our* code |
| the name-collision Python tool | Python | shares the `nlm` command name; a commonly recommended alternative |
| other NotebookLM MCP servers | mixed | enumerate at build time — the ecosystem moves; search first |
| official NotebookLM docs / API notes | — | if a public surface exists, sync it for the "what does the product actually do" baseline |

Do **not** hardcode the competitor list here — it goes stale. The build script
(below) discovers and prints what it synced; this doc records the *method*, not
a frozen roster.

### The blog posts

Each tool's launch/explainer posts are separate sources (they state *intended*
positioning, which is what we're comparing against, not just implemented
surface). Fetch as clean text:

```bash
curl -fsSL "$POST_URL" | html2md > "$WORKDIR/posts/<tool>-<slug>.md"
```

Bundle all posts into one `blog-posts` source via `nlm sync`.

## Build script (sketch — `~/bin/nlm-parity-notebook`)

Grounded on the real surface: `nlm notebook create` returns an id;
`nlm sync -n <name> <notebook-id> <path> [--exclude ...]` bundles a tree into one
named txtar source. `sync` auto-chunks at 5MB.

```bash
#!/usr/bin/env bash
set -euo pipefail
WORKDIR=$(mktemp -d ~/tmp/nlm-parity.XXXXXX)   # ~/tmp per house rules, not /tmp
NB=$(nlm notebook create "nlm parity analysis ($(date +%Y-%m-%d))")
echo "notebook: $NB"

# ghclone idiom: shallow clone, then sync the tree as one source.
ghclone_sync() {  # <name> <git-url>
  local name=$1 url=$2 dst="$WORKDIR/repos/$name"
  git clone --depth 1 "$url" "$dst"
  nlm sync -n "$name" "$NB" "$dst" \
    --exclude '.git/' --exclude 'vendor/' --exclude 'node_modules/' \
    --exclude 'testdata/' --exclude '*.png' --exclude '*.jpg'
}

ghclone_sync nlm-go       https://github.com/tmc/nlm
ghclone_sync nlm-python   <verify-the-real-url-first>
# ...enumerate the rest at build time...

# blog posts → one source
mkdir -p "$WORKDIR/posts"
# curl | html2md each post URL into $WORKDIR/posts/, then:
nlm sync -n blog-posts "$NB" "$WORKDIR/posts"

echo "sources:"; nlm source list "$NB"
```

Notes:
- **Binary hygiene:** exclude images/binaries so the corpus is text NLM can
  actually index and cite. `.git/` and `vendor/`/`node_modules/` must be excluded
  or the sync bloats and dilutes retrieval.
- **txtar caveat (task #14):** `nlm sync` quotes embedded txtar markers on upload
  but nothing unquotes on retrieval, and archive directives can leak into cited
  text. For *this* use case that's
  cosmetic — we're reading prose answers, not round-tripping the archive — but be
  aware citations may show `unquote NAME` noise. Do not build the parity notebook
  as a correctness test of sync; it's an analysis corpus.
- **Auth:** requires a signed-in profile (`nlm auth login`). Auth expires
  non-interactively — if a sync 401s mid-run, re-auth and resume.

## The analysis pass

Once synced, drive the comparison with grounded chat. One prompt per dimension
keeps answers concrete and citations tight (same discipline as the go-team
review's per-lens calls):

```bash
nlm generate-chat --citations tail "$NB" \
  "Compare how each tool in these sources handles browser/headless
   authentication. For each tool cite the file(s) that implement it, or state
   'not found in sources' — do not infer capabilities that aren't in the code."
```

Dimensions to sweep (one call each):
1. **Auth** — headless vs copy-paste; token refresh; multi-account.
2. **Chat** — interactive/streaming; session persistence; source selection.
3. **Sources** — upload formats (PDF/text/URL); bundling; sync/refresh.
4. **Artifacts** — audio, flashcards, mindmaps, deep research; export formats.
5. **MCP** — server present? which tools? injection vs watch.
6. **Distribution** — single binary vs runtime/venv; CI-friendliness.
7. **Wire modeling depth** — how much of the protocol each tool actually models.

For each dimension, the grounded answer + citations becomes a row in the parity
matrix. **HARD RULE (from go-team-review discipline):** every "tool X does/doesn't
do Y" claim is verified against the actual synced source before it enters the
matrix — the model hallucinates, and a parity claim that isn't cite-backed is
worthless. If NLM says "not found," `grep` the cloned tree to confirm absence
before recording it.

## Output

Two artifacts:
1. **A verified dimension × tool matrix** — each cell citing the source that
   grounds it; the evidence base for the README capabilities positioning.
2. **A short synthesis** — where `nlm` is at parity, where it's genuinely behind,
   and where it's ahead (single static binary, deepest wire modeling, MCP + CLI +
   interactive in one artifact).

## Honesty guardrails

- The notebook is an *input to* analysis, not the analysis. NLM drafts; we verify.
- Record what was synced and when (repos move; a parity claim is only as fresh as
  the clone). The build script prints the source list — capture it.
- If a competitor genuinely does something better, say so in the matrix. The
  positioning story (static binary, wire depth) is strong enough that we don't
  need to fudge the comparison — and a fudged matrix would fail the same way the
  external reviewer's ungrounded read did.
