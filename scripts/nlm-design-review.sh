#!/bin/bash
# nlm-design-review.sh — Sync a codebase into a notebook and ask a
# Go-expert-team meta-prompt for a one-shot "where is the design off" report.
#
# Usage:
#   nlm-design-review.sh <repo-path> [--notebook <id>] [--name <name>]
#                        [--finder <finder>] [--prompt-file <path>]
#                        [--out <file>] [--sync-only] [--ask-only]
#
# Flags:
#   --notebook <id>     Reuse an existing notebook (skips create).
#   --name <name>       Notebook title when creating (default: "design-review: <basename>").
#   --prompt-file <p>   Use custom meta-prompt instead of built-in.
#   --out <file>        Write report to file (default: stdout).
#   --verify-format     Citation sidecar format: jsonl (default) | grep | sarif | github.
#   --sync-only         Sync sources then stop (no chat).
#   --ask-only          Skip sync, just run the prompt against --notebook.
#
# Citation grounding:
#   When --out and a repo are both set, the raw `--citations=json` stream is
#   saved next to the report. Native NotebookLM citations are resolved back to
#   repo-relative file:line:col using the server-indexed source text. The
#   resolved sidecar format is controlled by --verify-format. If native
#   resolution fails, the older prose-level verifier is used as a fallback.
#
# Requires: nlm.

set -euo pipefail

REPO=""
NOTEBOOK_ID=""
NAME=""
PROMPT_FILE=""
OUT=""
VERIFY_FORMAT="jsonl"
SYNC_ONLY=false
ASK_ONLY=false
CHAT_JSONL=""

while [ $# -gt 0 ]; do
    case "$1" in
        --notebook)      NOTEBOOK_ID="$2"; shift 2 ;;
        --name)          NAME="$2"; shift 2 ;;
        --prompt-file)   PROMPT_FILE="$2"; shift 2 ;;
        --out)           OUT="$2"; shift 2 ;;
        --verify-format) VERIFY_FORMAT="$2"; shift 2 ;;
        --sync-only)     SYNC_ONLY=true; shift ;;
        --ask-only)      ASK_ONLY=true; shift ;;
        --help|-h)       sed -n '2,/^$/s/^# //p' "$0"; exit 0 ;;
        -*)              echo "unknown flag: $1" >&2; exit 1 ;;
        *)               REPO="$1"; shift ;;
    esac
done

if [ -z "$REPO" ] && ! $ASK_ONLY; then
    echo "error: repo path required" >&2
    exit 1
fi

if $ASK_ONLY && [ -z "$NOTEBOOK_ID" ]; then
    echo "error: --ask-only requires --notebook <id>" >&2
    exit 1
fi

if [ -n "$REPO" ]; then
    REPO=$(cd "$REPO" && pwd)
    [ -d "$REPO" ] || { echo "error: not a directory: $REPO" >&2; exit 1; }
fi

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
MODULE_ROOT=$(cd "$SCRIPT_DIR/.." && pwd)

# ── Create notebook if needed ────────────────────────────────────────

if [ -z "$NOTEBOOK_ID" ]; then
    [ -n "$NAME" ] || NAME="design-review: $(basename "$REPO")"
    echo "Creating notebook: $NAME"
    create_out=$(nlm create "$NAME" 2>&1)
    NOTEBOOK_ID=$(printf '%s' "$create_out" | grep -Eo '[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}' | head -1)
    if [ -z "$NOTEBOOK_ID" ]; then
        echo "error: could not extract notebook id from: $create_out" >&2
        exit 1
    fi
    echo "Notebook ID: $NOTEBOOK_ID"
fi

# ── Sync codebase ────────────────────────────────────────────────────

if ! $ASK_ONLY; then
    echo "═══════════════════════════════════════════════════"
    echo "  Syncing $REPO → $NOTEBOOK_ID"
    echo "═══════════════════════════════════════════════════"

    # `nlm source sync` handles chunking, hashing, and skip-on-unchanged itself.
    # Default source name is derived from the path basename.
    (cd "$REPO" && nlm source sync "$NOTEBOOK_ID" .)

    echo ""
    echo "Sources in notebook:"
    nlm source list "$NOTEBOOK_ID" | sed 's/^/  /' || true
fi

if $SYNC_ONLY; then
    echo ""
    echo "Sync complete. Notebook: $NOTEBOOK_ID"
    exit 0
fi

# ── Build meta-prompt ────────────────────────────────────────────────

if [ -n "$PROMPT_FILE" ]; then
    PROMPT=$(cat "$PROMPT_FILE")
else
    PROMPT=$(cat <<'EOF'
You are a review panel of senior Go engineers (think Russ Cox, Rob Pike,
Bryan Mills, Ian Lance Taylor). Your job is to read the codebase I have
uploaded as sources and identify where the design is off. No praise, no
restating what the code does, no filler.

Produce a report with these sections, in this order:

1. Top design smells
   Up to 8 bullets. Each bullet names the file or package and states the
   smell in one sentence. Rank by severity. Skip anything minor.

2. Interface and package shape
   Where are interfaces too wide, packages too entangled, or the dep
   graph inverted? Quote the offending type signatures.

3. Error handling and invariants
   Places where errors are swallowed, panics substitute for returns,
   or invariants are documented in prose but not enforced in code.

4. Concurrency hazards
   Shared state without synchronization, goroutines without lifecycles,
   channels used where sync primitives would be clearer.

5. Testability gaps
   APIs that cannot be tested without the network, filesystem, or
   time. Suggest the minimal refactor to fix each.

6. The one change I would make first
   If you could only change one thing, what and why. One paragraph.

Rules:
- Ground every claim in the uploaded sources. If you cannot ground it, omit it.
- Name concrete files or packages in prose when it helps readability, but do
  not fabricate file:line citations or source lists. Native NotebookLM
  citations are captured separately.
- Prefer "this is wrong because X" over "consider Y".
- No section headers beyond the six above. No emoji. No hedging.
EOF
)
fi

# ── Run the review ───────────────────────────────────────────────────

echo ""
echo "═══════════════════════════════════════════════════"
echo "  Running design review against $NOTEBOOK_ID"
echo "═══════════════════════════════════════════════════"

if [ -n "$OUT" ]; then
    CHAT_JSONL="${OUT%.md}.chat.jsonl"
    nlm generate-chat --citations=json "$NOTEBOOK_ID" "$PROMPT" \
        | tee "$CHAT_JSONL" \
        | (cd "$MODULE_ROOT" && go run ./internal/cmd/designreview render > "$OUT")
    echo "Report written to: $OUT" >&2
    echo "Chat JSONL written to: $CHAT_JSONL" >&2
else
    nlm generate-chat --citations=json "$NOTEBOOK_ID" "$PROMPT" \
        | (cd "$MODULE_ROOT" && go run ./internal/cmd/designreview render)
fi

# ── Citation grounding (when we have both a repo and a saved report) ───────
VERIFY_OUT=""
if [ -n "$REPO" ] && [ -n "$OUT" ]; then
    case "$VERIFY_FORMAT" in
        jsonl|"") ext="verification.jsonl" ;;
        grep)     ext="qf" ;;
        sarif)    ext="sarif" ;;
        github)   ext="github" ;;
        *)        echo "error: unknown --verify-format: $VERIFY_FORMAT" >&2; exit 1 ;;
    esac
    VERIFY_OUT="${OUT%.md}.$ext"
    VERIFY_COUNTS="${OUT%.md}.verification.jsonl"
    # Emit the requested format, plus a JSONL sidecar for summary counts.
    if [ -n "$CHAT_JSONL" ] \
        && (cd "$MODULE_ROOT" && \
        go run ./internal/cmd/designreview resolve --notebook "$NOTEBOOK_ID" --format "$VERIFY_FORMAT" < "$CHAT_JSONL" > "$VERIFY_OUT" 2>/dev/null) \
        && (cd "$MODULE_ROOT" && \
        go run ./internal/cmd/designreview resolve --notebook "$NOTEBOOK_ID" --format jsonl < "$CHAT_JSONL" > "$VERIFY_COUNTS" 2>/dev/null); then
        total=$(wc -l < "$VERIFY_COUNTS" | tr -d ' ')
        ok=$(grep -c '"status":"ok"' "$VERIFY_COUNTS" || true)
        header_span=$(grep -c '"status":"header_span"' "$VERIFY_COUNTS" || true)
        offset_miss=$(grep -c '"status":"offset_miss"' "$VERIFY_COUNTS" || true)
        {
            echo ""
            echo "── native citation grounding ─────────────────────────────────"
            echo "  $total citations resolved"
            echo "  ok: $ok   header-span: $header_span   offset-miss: $offset_miss"
            echo "  resolved citations ($VERIFY_FORMAT): $VERIFY_OUT"
            case "$VERIFY_FORMAT" in
                grep)   echo "  vim:    :cfile $VERIFY_OUT" ;;
                sarif)  echo "  upload: gh code-scanning upload-sarif --sarif $VERIFY_OUT" ;;
                github) echo "  emit:   cat $VERIFY_OUT   # run inside a GitHub Actions step" ;;
            esac
            if [ "$header_span" -gt 0 ] || [ "$offset_miss" -gt 0 ]; then
                echo "  warning: some native citations did not land cleanly inside a source body"
            fi
        } >&2
    elif (cd "$MODULE_ROOT" && \
        go run ./internal/cmd/designreview verify --repo "$REPO" --format "$VERIFY_FORMAT" < "$OUT" > "$VERIFY_OUT" 2>/dev/null) \
        && (cd "$MODULE_ROOT" && \
        go run ./internal/cmd/designreview verify --repo "$REPO" --format jsonl < "$OUT" > "$VERIFY_COUNTS" 2>/dev/null); then
        total=$(wc -l < "$VERIFY_COUNTS" | tr -d ' ')
        ok=$(grep -c '"status":"ok"' "$VERIFY_COUNTS" || true)
        miss_file=$(grep -c '"status":"file_miss"' "$VERIFY_COUNTS" || true)
        miss_line=$(grep -c '"status":"line_miss"' "$VERIFY_COUNTS" || true)
        ambiguous=$(grep -c '"status":"ambiguous"' "$VERIFY_COUNTS" || true)
        {
            echo ""
            echo "── fallback prose citation verification ──────────────────────"
            echo "  $total citations extracted"
            echo "  ok: $ok   file-missing: $miss_file   line-miss: $miss_line   ambiguous: $ambiguous"
            echo "  verifier output ($VERIFY_FORMAT): $VERIFY_OUT"
            if [ "$miss_file" -gt 0 ] || [ "$miss_line" -gt 0 ] || [ "$ambiguous" -gt 0 ]; then
                echo "  warning: prose citations did not fully verify"
            fi
        } >&2
    else
        echo "warning: native citation resolver and fallback verifier both failed" >&2
    fi
fi

# ── Exit hints (stderr) ──────────────────────────────────────────────
# `nlm generate-chat` already prints a `continue with:` line to stderr.
# Add the resync and rerun hints so all three are side by side.
{
    echo ""
    echo "── follow-ups ─────────────────────────────────────────────────"
    if [ -n "$REPO" ]; then
        echo "  resync sources:  (cd $REPO && nlm source sync $NOTEBOOK_ID .)"
    else
        echo "  resync sources:  (cd <repo> && nlm source sync $NOTEBOOK_ID .)"
    fi
    echo "  rerun review:    $0 --ask-only --notebook $NOTEBOOK_ID${OUT:+ --out $OUT}"
    echo "  open notebook:   https://notebooklm.google.com/notebook/$NOTEBOOK_ID"
} >&2
