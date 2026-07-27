---
title: MCP Server Comparison — nlm vs the NotebookLM MCP ecosystem
---
# MCP Server Comparison

Status: evidence · 2026-07-27 · Companion to [parity-matrix.md](parity-matrix.md) · Method: [parity-notebook.md](parity-notebook.md)

Focused comparison of the **MCP server surface** across the tools that ship one.
Grounded against synced source, every load-bearing claim re-verified against the
cloned trees (tool counts, transports, and primitives were grepped, not trusted).

Two of the seven tools ship **no MCP server** and are excluded here:
`notebooklm-cli-py` (legacy CLI) and `notebooklm-cli-go-dokkabei` (Go CLI).

## Servers in the field

| Server | Lang | Tools | Transport | Verified |
|---|---|---|---|---|
| **tmc-nlm-go** (`internal/nlmmcp`) | Go | **38** | stdio only | 24 direct + 14 generated tools, `mcp.StdioTransport{}` |
| notebooklm-mcp-cli-py (jacob-bd) | Python | **43** | stdio / http / sse | `@logged_tool`×43 |
| notebooklm-mcp-m4ykel | TypeScript | ~27 | stdio only | (down from 32; 5 removed in 0.3.1 for INVALID_ARGUMENT) |
| notebooklm-mcp-pleaseprompto | TypeScript | ~12 | stdio / streamable-http | DOM-driven; **only one with Resources** |
| notebooklm-mcp-alfredang | Python | 15 | stdio only | `@mcp.tool`×15; delegates to notebooklm-py |

## Quality matrix

Legend: ✅ · ➖ partial · ❌ absent.

| Capability | tmc-nlm-go | jacob-bd | m4ykel | pleaseprompto | alfredang |
|---|---|---|---|---|---|
| Direct-RPC backend (not DOM) | ✅ | ✅ | ✅ | ❌ DOM | ➖ delegated |
| Structured/typed output | ✅ proto→JSON | ✅ pydantic | ✅ zod | ✅ envelopes | ➖ dict |
| Error surfaced with `IsError` | ✅ | ✅ + `hint` | ✅ | ✅ typed classes | ➖ default |
| Non-stdio transport (http/sse) | ❌ | ✅ | ❌ | ✅ | ❌ |
| Long-op progress / async polling | ❌ poll-only | ➖ async query status | ❌ | ✅ progress cb | ❌ |
| **Resource / prompt primitives** | ❌ | ❌ | ❌ | ✅ **only one** | ❌ |
| Auth tools in-MCP (refresh/save) | ❌ (CLI `nlm auth`) | ✅ refresh/save | ✅ refresh/save | ✅ setup/re-auth/health | ❌ |
| Context injection (text/note) | ✅ | ✅ | ✅ | ✅ | ✅ |
| Local **file-path** injection | ❌ text-only | ✅ + allowlist | ✅ CWD/tmp-scoped | ❌ | ❌ |
| Deep-research start+poll tools | ✅ | ✅ | ✅ | ❌ | ❌ |
| Sharing tools | ❌ | ✅ (4) | ❌ | ❌ | ❌ |
| Batch / pipeline / cross-notebook | ❌ | ✅ | ❌ | ❌ | ❌ |

## Our 38 tools (verified in `internal/nlmmcp/tools.go`)

notebook: `list_notebooks` `create_notebook` `delete_notebook` · source:
`list_sources` `add_source_text` `add_source_url` `delete_source` · note:
`list_notes` `read_note` `create_note` `delete_note` · chat: `generate_chat` ·
artifact: `list_artifacts` `create_audio_overview` `create_video_overview`
`get_audio_overview` `rename_artifact` `share_audio` `create_slide_deck`
`create_app_artifact` · instructions: `set_instructions` `get_instructions` ·
research: `start_deep_research` `poll_deep_research` · generation:
`generate_summarize` `generate_briefing_doc` `generate_faq`
`generate_study_guide` `generate_rephrase` `generate_expand`
`generate_critique` `generate_brainstorm` `generate_verify`
`generate_explain` `generate_outline` `generate_mindmap`
`generate_timeline` `generate_toc`.

## Where we're competitive

- **Backend honesty.** Our server rides the same lossless-verified proto codec as
  the CLI (`beprotojson`), so tool results are typed proto→JSON, not scraped DOM
  text (pleaseprompto) or hand-indexed arrays. Same moat as the CLI side.
- **Context injection works** — `add_source_text` + `create_note` cover the
  "inject a git diff / scratch note" agent use case the external reviewer rated
  important. This is **not** a gap (reviewer was wrong).
- **Deep research is wired** — `start_deep_research` + `poll_deep_research` let an
  agent kick off and track a long research job.

## Where we're behind (verified — real MCP gaps)

1. **Breadth: 38 vs 43 tools.** jacob-bd's server exposes notebook sharing, batch,
   pipeline, cross-notebook query, and async query-status that we don't. Not all
   are worth copying, but **sharing** and **async/long-op status** are the two
   most defensible adds.
2. **stdio-only transport.** jacob-bd and pleaseprompto both offer HTTP/SSE;
   we're stdio-only (`server.go:51`). Fine for local agents, limiting for
   remote/hosted agent setups.
3. **No progress notifications.** The Go MCP SDK supports them; we wire none.
   Deep research is start+poll, not push. pleaseprompto has real progress
   callbacks. (This is spec **B5**.)
4. **No in-MCP auth tools.** Every other RPC server exposes `refresh_auth` /
   `save_auth_tokens` so the agent can recover from an expired session without
   dropping to a shell; ours requires the human to run `nlm auth`. Ties directly
   to the auth-refresh gap (spec **B2**) — worth an `refresh_auth` MCP tool.
5. **No local file-path injection.** Our MCP exposes `add_source_text` /
   `add_source_url` but not a file-path upload tool (the CLI has `source add`);
   jacob-bd and m4ykel both allow path reads (behind directory allowlists). A
   path-scoped `add_source_file` MCP tool would close this — with the same
   allowlist guard they use.

## The one thing nobody has that we could own

**Resource primitives.** pleaseprompto is the *only* server exposing MCP
Resources (`notebooklm://library/...`) and prompt templates — everyone else,
us included, exposes tools only. Exposing notebooks/sources/notes as MCP
**resources** (readable URIs an agent can browse without a tool call) would be a
differentiated, protocol-native move that fits our "deepest modeling" identity.

## Recommended MCP roadmap (ordered)

| Pri | Add | Rationale |
|---|---|---|
| P1 | `refresh_auth` MCP tool | agents recover from 401 without a shell; pairs with spec B2 |
| P1 | progress notifications on deep research | spec B5; SDK supports it; pleaseprompto proves the pattern |
| P2 | `add_source_file` (path, allowlisted) | close the file-injection gap safely |
| P2 | sharing tools | jacob-bd has 4; common agent ask |
| P3 | HTTP/SSE transport option | unlocks remote/hosted agents |
| P3 | notebooks/sources as MCP Resources | protocol-native, unclaimed niche |

## Bundled agent skills (Claude Code / Codex / Gemini)

Most of these tools ship an **agent skill** (a `SKILL.md` that teaches Claude
Code / Codex / Gemini how to drive the tool). Read directly from the trees — not
RAG — so these are exact.

| Tool | Skill file | Lines | Style | Installer | AI-docs flag |
|---|---|---|---|---|---|
| **tmc-nlm-go** | `skills/nlm/SKILL.md` + `reference/commands.md` | 196 + 218 | **lean, delegates to live `--help`** | ❌ in-repo only | ❌ |
| notebooklm-mcp-cli-py (jacob-bd) | `data/SKILL.md` | **921** | exhaustive, MCP+CLI dual-mode w/ detection logic | ✅ `nlm skill` → `~/.agents/skills`, codex, gemini-cli, antigravity | ✅ `nlm --ai` |
| notebooklm-cli-py (jacob-bd) | `nlm-cli-skill/SKILL.md` (+ `.zip`) | ~mid | CLI-only expert guide | ➖ ships a zip | ✅ `nlm --ai` |
| notebooklm-cli-go-dokkabei | `skills/notebooklm/SKILL.md` | 135 | workflow-first (source→ask→research) | ❌ in-repo | ❌ |
| notebooklm-mcp-alfredang | `SKILL.md` | short | content→format table | ❌ | ❌ |
| m4ykel / pleaseprompto | `CLAUDE.md`/`GEMINI.md` only (no SKILL) | — | agent instructions, not a skill | ❌ | ❌ |

### Read on the skills

- **Ours is deliberately lean and self-updating.** `skills/nlm/SKILL.md`
  delegates to live `nlm --help` / `nlm <cmd> --help` as the source of truth and
  keeps only a compact `reference/commands.md` — so the skill can't rot as the
  CLI evolves. It documents our differentiators well (sync/txtar, source-match,
  UUID surfacing, `-y` for destructive ops). Good design; small footprint.
- **jacob-bd out-invests everyone on agent onboarding.** A 921-line skill with
  explicit "check for MCP tools vs CLI, and *ask the user which*" branching, plus
  two things we lack entirely:
  1. **`nlm --ai`** — a flag that dumps AI-optimized full docs in one shot (the
     skill tells the agent to run it first). We have no equivalent; an agent must
     crawl `--help` per command.
  2. **A skill installer** (`nlm skill`) that deploys the skill into
     `~/.agents/skills/`, Codex, Gemini-CLI, and Antigravity project dirs. Ours
     is copy-it-yourself.
- **Everyone else is thinner than us** — dokkabei/alfredang have short
  workflow skills; the two TS MCP servers ship only `CLAUDE.md`/`GEMINI.md`
  agent-instruction files, not installable skills.

### Skills gaps worth closing (real, verified)

| Pri | Add | Rationale |
|---|---|---|
| P2 | `nlm --ai` one-shot AI docs dump | agents onboard in one call instead of crawling `--help`; jacob-bd proves the pattern |
| P3 | `nlm skill install` → `~/.agents/skills` / codex / claude | removes the copy-it-yourself friction for agent adoption |

Keep our lean, `--help`-delegating skill *style* — it's better engineered than
jacob-bd's 921-line monolith. The gaps are **distribution** (installer) and a
**one-call docs entrypoint** (`--ai`), not skill content.

## Provenance

Notebook `<notebook-id>`; raw transcripts at
a local working directory. Tool counts (38 / 43 / 15) and
transport claims verified by grep against the cloned trees, not taken from the
grounded answer on faith.
