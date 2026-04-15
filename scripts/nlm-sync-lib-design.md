# nlm-sync-lib.sh — Design Document

## Problem

17 nlm-sync scripts (7,333 lines) share ~200 lines of duplicated upload
infrastructure each.  Measured across all scripts, 46–60% of every file is
copy-pasted boilerplate: source cache, file finders, upload engine, CLI
arg parsing.  The copies have diverged, producing five distinct upload
engine implementations with different bugs.

### Bugs in the wild

| # | Bug | Where | Impact |
|---|-----|-------|--------|
| 1 | **"Pasted Text" grep rename** — finds newly uploaded source by grepping for the name "Pasted Text".  Races if two uploads are in flight or another NLM user adds a source concurrently. | 9 scripts (gen-1) | Wrong source gets renamed; correct source stays as "Pasted Text" forever |
| 2 | **No content hashing** — without hashing, `--force` is the only way to update changed content.  Most scripts skip a source if its name exists, even if the files changed. | 15 of 17 scripts | Stale content until manual `--force` |
| 3 | **`2>&1 2>&1` double redirect** | modelir-v2.sh lines 136, 144 | stderr discarded silently |
| 4 | **Empty array + `set -u`** — `upload_dirs` passes `"${files[@]}"` without guarding for empty arrays.  Under `set -u` (all scripts), bash 3.2 treats this as an unbound variable. | mlx-go.sh, aistudio.sh, others | Script crashes on directories with no matching files |
| 5 | **`--force` deletes before uploading** in git-history | git-history.sh line 204 | Content briefly missing from notebook |
| 6 | **Inconsistent DELAY** — ranges from 0 to 3 seconds per chunk | all | Unnecessarily slow syncs on newer scripts that forgot to change the default |
| 7 | **Inconsistent error filtering** — some grep `"invalid character"`, some grep `"^DEBUG"`, some grep both | all | Noisy or swallowed errors depending on script vintage |

### Existing nlm features (discovered during audit)

The nlm CLI already has features that eliminate several workarounds:

- **`nlm add --name <title>`** — already works for stdin/file/text.
  Eliminates the entire "Pasted Text" grep + rename dance across all 9
  gen-1 scripts and the `comm -13` detection across 5 gen-2 scripts.
- **Source ID returned on stdout** — `nlm add` already prints the new
  source UUID.  A few status messages leak to stdout (being fixed to
  stderr) so `new_id=$(nlm add ...)` works cleanly.

Being added to nlm (coordinated with nlm repo maintainer):

- **`nlm add --replace <source-id>`** — atomic gap-free replacement.
  Upload new, rename, delete old.  Old source untouched if upload fails.
- **`nlm sources --json`** — machine-readable source listing.

## Goals

1. **One canonical upload engine** sourced by all scripts.
2. **Content hashing everywhere** — detect changes automatically, no `--force` for routine syncs.
3. **Use `nlm add --name`** everywhere — eliminates rename-after-upload entirely.
4. **Gap-free updates** via `nlm add --replace` (when available) or rename-old → upload → delete-old.
5. **Fix all known bugs** listed above.
6. **Minimal migration** — each script sources the lib and keeps only its content definitions.

## Non-goals

- Rewriting `nlm` itself (the CLI is a given).
- Adding features that no script currently needs.
- Changing the txtar bundling format.

## Architecture

```
~/bin/nlm-sync-lib.sh          ← shared library (source'd, never executed directly)
~/bin/nlm-sync-<project>.sh    ← per-project scripts (content definitions only)
~/.cache/nlm-sync/<notebook>/  ← content hash cache (per notebook)
```

A refactored script looks like:

```bash
#!/bin/bash
# nlm-sync-foo.sh — Synchronize foo to NotebookLM.
set -uo pipefail

REPO="${FOO_ROOT:-$HOME/foo}"
source "${NLM_SYNC_LIB:-$HOME/bin/nlm-sync-lib.sh}"

nlm_sync_parse_args "$@"       # sets NOTEBOOK_ID, FORCE, ONLY, etc.
nlm_sync_require_dir "$REPO"
nlm_sync_begin "nlm-sync-foo" "root=$REPO"

upload_dirs "foo core" "$REPO/src"
upload_py_dirs "foo tests" "$REPO/tests"
upload_files "foo config" "$REPO/pyproject.toml" "$REPO/README.md"

nlm_sync_end
```

## Library API

### Initialization

```bash
source ~/bin/nlm-sync-lib.sh
```

Sets default values for all configuration variables.  Does NOT call
`set -euo pipefail` (that's the caller's job — the lib must work under
the caller's shell options).

Fails hard with a clear error if `nlm`, `txtar-c`, and `shasum` are not
on PATH.

### Configuration variables

Set these before calling `nlm_sync_parse_args` to override defaults:

| Variable | Default | Description |
|----------|---------|-------------|
| `NLM_SYNC_MAX_BYTES` | `450000` | Chunk split threshold |
| `NLM_SYNC_DELAY` | `0` | Seconds to sleep after each upload |
| `NLM_SYNC_MAX_PARTS` | `20` | Maximum part number to check in `--list` mode |
| `NLM_SYNC_HASH_DIR` | `~/.cache/nlm-sync/$NOTEBOOK_ID` | Content hash cache directory |

### CLI parsing

```bash
nlm_sync_parse_args "$@"
```

Parses the standard flag set and sets globals:

| Global | Type | Source |
|--------|------|--------|
| `NOTEBOOK_ID` | string | positional arg |
| `FORCE` | bool | `--force` |
| `ONLY` | string | `--only <filter>` |
| `LIST_ONLY` | bool | `--list` |
| `DRY_RUN` | bool | `--dry-run` |
| `TARGET_FILE` | string | `--file <path>` |

Scripts that need custom flags (e.g., `--all-mlx-go`, `--branch`,
`--stat`) parse their own flags first, consume them from `$@`, then pass
the remainder to `nlm_sync_parse_args`.

### Lifecycle

```bash
nlm_sync_require_dir "$path"           # exit 1 if dir missing
nlm_sync_begin "script-name" "key=val" # banner + refresh_sources_cache
nlm_sync_end                           # footer
```

### Source cache

```bash
refresh_sources_cache    # fetch current source list from NLM
source_exists "name"     # grep -qF check against cache
source_id_for "name"     # extract UUID for a named source
```

Identical to every script today.  One copy.

### Content hashing

```bash
_nlm_hash_init                          # mkdir -p $NLM_SYNC_HASH_DIR
_nlm_content_hash "$content"            # shasum -a 256, returns 64-char hex
_nlm_cached_hash "source-name"          # read cached hash, empty if none
_nlm_save_hash "source-name" "$hash"    # write hash to cache
_nlm_content_changed "name" "$hash"     # returns 0 if changed or no cache
```

Content hashing is **always on**.  The decision tree in `_upload_chunk`:

```
source exists in NLM?
├─ no  → upload (new source)
└─ yes
   ├─ --force → upload (replace)
   └─ content hash matches cached hash?
      ├─ yes → SKIP (unchanged)
      └─ no  → upload (changed)
          └─ no cached hash? → SKIP with "(exists, no hash — use --force)"
```

This matches the ane script's behavior, generalized.  The "no cached hash"
case handles the first run after adding hashing to an existing notebook —
it won't re-upload everything, but will tell you to `--force` once.

### Upload engine

One `_upload_chunk` implementation combining the best of all generations:

```bash
_upload_chunk "name" part_number "txtar_ok" file1 file2 ...
```

Algorithm (with `--name` and `--replace` support):

1. **Build txtar content** — via `echo | txtar` or `txtar-c -quote` tmpdir.
2. **Compute content hash** of the txtar output.
3. **Check skip conditions**:
   - Empty content → return.
   - Source exists + not forced + hash unchanged → SKIP.
   - Source exists + not forced + no cached hash → SKIP with hint.
4. **Upload with naming** — two strategies depending on gap-free needs:

   **Fast path** (`--replace`, brief overlap):
   ```bash
   new_id=$(nlm add "$NOTEBOOK_ID" - --name "$part_name" --replace "$old_id" < content)
   ```
   Uploads new, deletes old.  Brief moment where both exist but old name
   is gone.  Good enough for most syncs.

   **Gap-free path** (zero-downtime, used when content must never be missing):
   ```bash
   nlm rename-source "$old_id" "$part_name [old]"    # park old
   new_id=$(nlm add "$NOTEBOOK_ID" - --name "$part_name" < content)
   nlm --yes rm-source "$NOTEBOOK_ID" "$old_id"      # remove old
   ```
   Old source stays accessible under "[old]" name until new one is live.

   **New source** (no old to replace):
   ```bash
   new_id=$(nlm add "$NOTEBOOK_ID" - --name "$part_name" < content)
   ```

   In all cases the source ID comes back on stdout — no `comm -13` or
   "Pasted Text" grep needed.

5. **Save content hash**.
6. **Refresh source cache**.

**Note on `--replace` semantics** (from nlm maintainer): The NLM API has
no in-place source content update.  `MutateSource` only changes metadata.
So `--replace` is CLI orchestration: add new + delete old.  The 4-step
gap-free dance is only needed if zero-downtime matters.

The `comm -13` pattern and "Pasted Text" grep are both eliminated by
using `--name` + capturing stdout.

**nlm changes** (commit d104ac3):
- `nlm add --name` — already existed, status messages moved to stderr
- `nlm add --replace <source-id>` — upload-first-then-delete
- `nlm sources --json` — returns `[{source_id, title, source_type, status, last_modified}]`

Error output filtering: consistently suppress `"invalid character"` and
`"^DEBUG"` messages via a single helper:

```bash
_nlm_filter_noise() { grep -v 'invalid character\|^DEBUG\|^Are you' || true; }
```

### Convenience uploaders

All call `upload_batch` internally.

```bash
upload_batch "name" file1 file2 ...       # core: chunk + upload
upload_dirs "name" dir1 dir2 ...          # find_go → upload_batch
upload_py_dirs "name" dir1 dir2 ...       # find_py → upload_batch
upload_swift_dirs "name" dir1 dir2 ...    # find_swift → upload_batch
upload_files "name" file1 file2 ...       # filter existing → upload_batch
upload_collected "name" < <(find ...)     # stdin → upload_batch
upload_text "name" /path/to/file          # raw text (no txtar), with hashing
upload_text_chunked "name" /path/to/file  # raw text, split at heading boundaries
```

`upload_batch` handles:
- `--only` filtering
- `--list` mode (check existence, print status)
- `--file` targeting (skip bundles not containing target file, force-upload matching bundle)
- Empty file list → "SKIP (no files)" (fixes bug #4)
- txtar compatibility check
- Chunking at `NLM_SYNC_MAX_BYTES`

**Empty array guard** (fixes bug #4):

```bash
upload_batch() {
    local name="$1"; shift
    # ...
    local files=()
    if [ $# -gt 0 ]; then
        files=("$@")
    fi
    if [ ${#files[@]} -eq 0 ]; then
        echo "  SKIP $name (no files)"
        return
    fi
    # ...
}
```

### File finders

```bash
find_go "$dir"          # *.go *.md, excludes .git vendor testdata .build .gomodcache
find_go_only "$dir"     # *.go only
find_py "$dir"          # *.py *.md, excludes __pycache__ venv build dist egg-info
find_swift "$dir"       # *.swift, excludes .git .build Tests
find_src "$dir"         # *.go *.md *.proto *.metal, broad exclusions
find_src_broad "$dir"   # everything sol.sh needs (*.go *.rs *.ts *.py *.c ...), maximal exclusions
```

Each finder returns sorted, newline-delimited absolute paths.  Scripts can
define custom finders and pass them to `upload_dirs` via a variable:

```bash
NLM_SYNC_FINDER=find_py upload_dirs "name" "$dir"
```

Or just use `upload_collected` with a custom find pipeline.

## Migration plan

### Phase 1: Write the library

Create `~/bin/nlm-sync-lib.sh` with all shared functions.  Write a test
harness that validates:
- `nlm_sync_parse_args` sets correct globals for all flag combos
- `_nlm_content_hash` produces stable hashes
- `_nlm_content_changed` returns correct results
- Empty file list doesn't crash under `set -u`
- Chunk splitting produces correct part counts

### Phase 2: Migrate one script

Refactor `nlm-sync-mlx-audio.sh` (smallest, cleanest, gen-2 already) to
source the lib.  Run both old and new with `--dry-run` and diff the
output.  Verify identical upload plan.

### Phase 3: Migrate remaining scripts

In order of complexity:
1. **Trivial** (identical engine, just content defs): mlx-video, mlx-vlm,
   mlx-embeddings, mlx-py, mlx-lm-py, aistudio, mlx-transformer-vm
2. **Moderate** (gen-1 engine, needs `comm -13` upgrade): mlx-go, nlm,
   snes, vz-macos
3. **Has extra features**: mlx-go-ane (`--file`, already has hashing),
   modelir-v2 (`--all-mlx-go`, multi-language finders)
4. **Unique upload patterns**: claude-sessions (JSONL rendering, text
   upload, Codex discovery), git-history (raw text, line-based splitting)
5. **Parallel architecture**: sol (manifest queue, xargs concurrency,
   retry, cleanup — may stay mostly custom but source the lib for cache +
   finders + hashing)

### Phase 4: Delete dead code

Remove the inline infrastructure from each migrated script.  Each script
should be < 50% of its original size.

## Special cases

### sol.sh — parallel uploads

Sol's `__upload` subprocess pattern and `xargs -P` concurrency are
orthogonal to the lib.  Sol should:
- Source the lib for: `refresh_sources_cache`, `source_exists`,
  `source_id_for`, `find_src_broad`, `_nlm_filter_noise`
- Keep its own: `queue_repo`, `add_manifest`, `__upload` subprocess,
  `cleanup_notebook`, `git_sync`
- Adopt content hashing inside `__upload` (check hash before uploading)
- Adopt `comm -13` detection inside `__upload` (replace "Pasted Text" grep
  — but note sol extracts the ID from `nlm add` stdout via UUID regex,
  which is actually better than both grep and comm when it works)

### claude-sessions.sh — session rendering

Claude-sessions should:
- Source the lib for: source cache, content hashing, `_nlm_filter_noise`,
  `nlm_sync_parse_args` (extended with its custom flags)
- Keep its own: `render_session` (Python renderer), `upload_text`,
  `upload_text_chunked`, `upload_txtar_dir`, Codex session discovery

### git-history.sh — raw text upload

Git-history should:
- Source the lib for: source cache, `nlm_sync_parse_args` (extended),
  `_nlm_filter_noise`
- Adopt content hashing (hash the git log output)
- Adopt gap-free updates (its `--force` currently deletes before uploading)
- Keep its own: `git_log_stat`, `git_log_full`, `upload_log_output`,
  line-based `_upload_chunk`

## Testing

### Test harness: `nlm-sync-lib-test.sh`

Unit tests using a mock `nlm` command:

```bash
# Mock nlm: records calls, returns canned responses
mock_nlm() {
    echo "$*" >> "$TEST_DIR/nlm_calls.log"
    case "$1" in
        sources) cat "$TEST_DIR/mock_sources.txt" ;;
        add)     echo "source-new-uuid-1234" ;;
        # ...
    esac
}
alias nlm=mock_nlm
```

Test cases:
1. **parse_args**: all flag combinations
2. **content hashing**: stable output, cache hit/miss/changed
3. **empty file list**: no crash under `set -u`
4. **chunk splitting**: file list at boundary sizes
5. **skip logic**: exists + unchanged, exists + changed, exists + no hash, missing
6. **gap-free update**: verify rename-old → upload → rename-new → delete-old ordering
7. **comm -13 detection**: verify correct ID extracted
8. **--only filtering**: matching and non-matching names
9. **--list mode**: correct status output
10. **--dry-run**: no nlm mutations

## Size estimate

| Component | Lines |
|-----------|-------|
| `nlm-sync-lib.sh` | ~350 |
| Test harness | ~200 |
| Refactored per-script (average) | ~50% reduction |
| Total lines saved | ~3,000 of 7,333 |

## Open questions

1. **Hash cache cleanup** — `~/.cache/nlm-sync/<notebook>/` accumulates
   hash files forever.  Should the lib prune entries for source names that
   no longer exist in the notebook?  Low priority but prevents unbounded
   growth.

2. **`nlm sources --json` field stability** — once scripts parse JSON
   instead of grepping table output, the field names become a contract.
   Need to confirm the schema with the nlm maintainer before relying on it.

## Resolved questions

1. ~~`nlm add --name`~~ — **Already exists**.  Discovered during
   cross-repo collaboration.  Eliminates rename-after-upload entirely.

2. ~~Source ID on stdout~~ — **Already works**.  `nlm add` prints the UUID.
   Status message noise being fixed to stderr in nlm repo.

3. ~~Sol's UUID extraction~~ — Sol's approach of parsing the UUID from
   stdout is correct.  With the stderr cleanup, this becomes the canonical
   pattern: `new_id=$(nlm add "$NOTEBOOK_ID" - --name "$name" < content)`.
