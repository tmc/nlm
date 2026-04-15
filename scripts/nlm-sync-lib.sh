#!/bin/bash
# nlm-sync-lib.sh — Shared library for nlm-sync scripts.
#
# Source this file; do not execute directly.
#
# Provides: source cache, content hashing, upload engine, file finders,
# CLI arg parsing, and convenience uploaders.
#
# Usage:
#   source "${NLM_SYNC_LIB:-$HOME/bin/nlm-sync-lib.sh}"
#   nlm_sync_parse_args "$@"
#   nlm_sync_require_dir "$REPO"
#   nlm_sync_begin "script-name" "root=$REPO"
#   upload_dirs "bundle name" "$REPO/src" "$REPO/lib"
#   nlm_sync_end
#
# Requirements: nlm, txtar-c, shasum, bash 3.2+

# Guard against direct execution.
if [ "${BASH_SOURCE[0]}" = "$0" ]; then
    echo "error: nlm-sync-lib.sh must be sourced, not executed" >&2
    exit 1
fi

# ── Dependency check ─────────────────────────────────────────────────

_nlm_check_deps() {
    local missing=()
    command -v nlm >/dev/null 2>&1    || missing+=(nlm)
    command -v txtar-c >/dev/null 2>&1 || missing+=(txtar-c)
    command -v shasum >/dev/null 2>&1  || missing+=(shasum)
    command -v jq >/dev/null 2>&1     || missing+=(jq)
    if [ ${#missing[@]} -gt 0 ]; then
        echo "error: nlm-sync-lib requires: ${missing[*]}" >&2
        return 1
    fi
}
_nlm_check_deps || return 1

# ── Configuration defaults ───────────────────────────────────────────

NLM_SYNC_MAX_BYTES="${NLM_SYNC_MAX_BYTES:-450000}"
NLM_SYNC_DELAY="${NLM_SYNC_DELAY:-0}"
NLM_SYNC_MAX_PARTS="${NLM_SYNC_MAX_PARTS:-20}"
NLM_SYNC_GAP_FREE="${NLM_SYNC_GAP_FREE:-false}"

# These are set by nlm_sync_parse_args.
NOTEBOOK_ID="${NOTEBOOK_ID:-}"
FORCE="${FORCE:-false}"
ONLY="${ONLY:-}"
LIST_ONLY="${LIST_ONLY:-false}"
DRY_RUN="${DRY_RUN:-false}"
TARGET_FILE="${TARGET_FILE:-}"

# ── CLI argument parsing ─────────────────────────────────────────────

nlm_sync_parse_args() {
    while [ $# -gt 0 ]; do
        case "$1" in
            --force)    FORCE=true; shift ;;
            --only)     ONLY="$2"; shift 2 ;;
            --list)     LIST_ONLY=true; shift ;;
            --dry-run)  DRY_RUN=true; shift ;;
            --file)     TARGET_FILE="$2"; shift 2 ;;
            --delay)    NLM_SYNC_DELAY="$2"; shift 2 ;;
            --max-bytes) NLM_SYNC_MAX_BYTES="$2"; shift 2 ;;
            --gap-free) NLM_SYNC_GAP_FREE=true; shift ;;
            --help|-h)  sed -n '2,/^$/s/^# //p' "${BASH_SOURCE[-1]}" 2>/dev/null; exit 0 ;;
            -*)         echo "unknown flag: $1" >&2; return 1 ;;
            *)          NOTEBOOK_ID="$1"; shift ;;
        esac
    done

    if [ -z "$NOTEBOOK_ID" ]; then
        echo "error: notebook ID required" >&2
        return 1
    fi

    # Resolve --file to absolute path.
    if [ -n "$TARGET_FILE" ]; then
        case "$TARGET_FILE" in
            /*) ;;
            *)  TARGET_FILE="$(pwd)/$TARGET_FILE" ;;
        esac
        if [ ! -f "$TARGET_FILE" ]; then
            echo "error: --file target does not exist: $TARGET_FILE" >&2
            return 1
        fi
        TARGET_FILE=$(cd "$(dirname "$TARGET_FILE")" && echo "$(pwd)/$(basename "$TARGET_FILE")")
    fi
}

# ── Lifecycle ────────────────────────────────────────────────────────

nlm_sync_require_dir() {
    if [ ! -d "$1" ]; then
        echo "error: directory does not exist: $1" >&2
        exit 1
    fi
}

nlm_sync_begin() {
    local script_name="$1"; shift
    echo "═══════════════════════════════════════════════════"
    echo "  $script_name: $NOTEBOOK_ID"
    local kv
    for kv in "$@"; do
        echo "  $kv"
    done
    echo "  force=$FORCE  only=${ONLY:-all}"
    echo "═══════════════════════════════════════════════════"
    echo ""
    refresh_sources_cache
    _nlm_hash_init
}

nlm_sync_end() {
    refresh_sources_cache
    echo ""
    echo "═══════════════════════════════════════════════════"
    echo "  Done. Verify: nlm sources $NOTEBOOK_ID"
    echo "═══════════════════════════════════════════════════"
}

# ── Source cache ─────────────────────────────────────────────────────

_sources_cache_json=""

refresh_sources_cache() {
    _sources_cache_json=$(nlm sources "$NOTEBOOK_ID" --json 2>/dev/null)
}

source_exists() {
    printf '%s' "$_sources_cache_json" | jq -e --arg t "$1" '.[] | select(.title == $t)' >/dev/null 2>&1
}

source_id_for() {
    printf '%s' "$_sources_cache_json" | jq -r --arg t "$1" '[.[] | select(.title == $t)][0].source_id // empty'
}

# ── Content hashing ──────────────────────────────────────────────────

_nlm_hash_dir=""

_nlm_hash_init() {
    _nlm_hash_dir="${NLM_SYNC_HASH_DIR:-$HOME/.cache/nlm-sync/$NOTEBOOK_ID}"
    mkdir -p "$_nlm_hash_dir"
}

_nlm_hash_key() {
    echo "$1" | shasum -a 256 | cut -c1-16
}

_nlm_cached_hash() {
    local key
    key=$(_nlm_hash_key "$1")
    [ -f "$_nlm_hash_dir/$key" ] && cat "$_nlm_hash_dir/$key"
}

_nlm_save_hash() {
    local key
    key=$(_nlm_hash_key "$1")
    echo "$2" > "$_nlm_hash_dir/$key"
}

# Returns 0 (true) if content changed or no cache exists.
_nlm_content_changed() {
    local name="$1" hash="$2"
    local prev
    prev=$(_nlm_cached_hash "$name")
    if [ -z "$prev" ]; then
        return 0  # no cache — treat as changed (caller decides skip vs upload)
    fi
    [ "$hash" != "$prev" ]
}

# ── Error output filter ─────────────────────────────────────────────

_nlm_filter_noise() {
    grep -v 'invalid character\|^DEBUG\|^Are you' || true
}

# ── Upload engine ────────────────────────────────────────────────────

_upload_chunk() {
    local name="$1" part="$2" txtar_ok="$3"
    shift 3

    # Guard empty file list (fixes set -u crash).
    if [ $# -eq 0 ]; then
        return
    fi
    local chunk_files=("$@")

    local part_name="$name"
    [ "$part" -gt 1 ] && part_name="$name (pt$part)"

    # ── Build content + measure size + compute hash ──
    local chunk_size="" content_hash="" tmpdir="" txtar_content=""

    if [ "$txtar_ok" = "true" ]; then
        txtar_content=$(echo | txtar "${chunk_files[@]}" 2>/dev/null)
        chunk_size=$(printf '%s' "$txtar_content" | wc -c | tr -d ' ')
        content_hash=$(printf '%s' "$txtar_content" | shasum -a 256 | cut -c1-64)
    else
        tmpdir=$(mktemp -d)
        local f
        for f in "${chunk_files[@]}"; do
            mkdir -p "$tmpdir/$(dirname "$f")"
            cp "$f" "$tmpdir/$f" 2>/dev/null || true
        done
        txtar_content=$(txtar-c -quote "$tmpdir" 2>/dev/null)
        chunk_size=$(printf '%s' "$txtar_content" | wc -c | tr -d ' ')
        content_hash=$(printf '%s' "$txtar_content" | shasum -a 256 | cut -c1-64)
    fi

    if [ "$chunk_size" -eq 0 ]; then
        [ -n "$tmpdir" ] && rm -rf "$tmpdir"
        return
    fi

    # ── Skip logic ──
    local old_id=""
    if source_exists "$part_name"; then
        if ! $FORCE; then
            local prev_hash
            prev_hash=$(_nlm_cached_hash "$part_name")
            if [ -n "$prev_hash" ] && [ "$content_hash" = "$prev_hash" ]; then
                echo "  SKIP $part_name (unchanged)"
                [ -n "$tmpdir" ] && rm -rf "$tmpdir"
                return
            fi
            if [ -z "$prev_hash" ]; then
                echo "  SKIP $part_name (exists, no hash — use --force)"
                [ -n "$tmpdir" ] && rm -rf "$tmpdir"
                return
            fi
            echo "  CHANGED $part_name (hash differs)"
        fi
        old_id=$(source_id_for "$part_name")
    fi

    # ── Dry run ──
    if $DRY_RUN; then
        echo "  WOULD UPLOAD $part_name (${chunk_size} bytes, ${#chunk_files[@]} files)"
        [ -n "$tmpdir" ] && rm -rf "$tmpdir"
        return
    fi

    # ── Upload ──
    echo "  UPLOAD $part_name (${chunk_size} bytes, ${#chunk_files[@]} files)"
    local new_id=""

    if [ -n "$old_id" ] && [ "$NLM_SYNC_GAP_FREE" = "true" ]; then
        # Gap-free path: rename old to "[old]" first so the name is never absent.
        # This is intentionally NOT using --replace, because --replace deletes
        # the old source after upload — there's a moment where the old name is
        # gone.  The gap-free path keeps the old content accessible under "[old]"
        # until the new upload is confirmed.
        nlm rename-source "$old_id" "$part_name [old]" 2>&1 | _nlm_filter_noise
        new_id=$(printf '%s' "$txtar_content" | nlm add "$NOTEBOOK_ID" - --name "$part_name" 2> >(_nlm_filter_noise >&2)) || {
            echo "  ERROR uploading $part_name" >&2
            [ -n "$tmpdir" ] && rm -rf "$tmpdir"
            return 1
        }
        echo "  REMOVE old $old_id ($part_name)"
        nlm --yes rm-source "$NOTEBOOK_ID" "$old_id" 2>&1 | _nlm_filter_noise
    elif [ -n "$old_id" ]; then
        # Fast path: --replace uploads new then deletes old.
        new_id=$(printf '%s' "$txtar_content" | nlm add "$NOTEBOOK_ID" - --name "$part_name" --replace "$old_id" 2> >(_nlm_filter_noise >&2)) || {
            echo "  ERROR uploading $part_name" >&2
            [ -n "$tmpdir" ] && rm -rf "$tmpdir"
            return 1
        }
    else
        # New source.
        new_id=$(printf '%s' "$txtar_content" | nlm add "$NOTEBOOK_ID" - --name "$part_name" 2> >(_nlm_filter_noise >&2)) || {
            echo "  ERROR uploading $part_name" >&2
            [ -n "$tmpdir" ] && rm -rf "$tmpdir"
            return 1
        }
    fi

    [ -n "$tmpdir" ] && rm -rf "$tmpdir"

    if [ -n "$NLM_SYNC_DELAY" ] && [ "$NLM_SYNC_DELAY" -gt 0 ] 2>/dev/null; then
        sleep "$NLM_SYNC_DELAY"
    fi

    # ── Save hash ──
    _nlm_save_hash "$part_name" "$content_hash"
    echo "  OK $part_name${new_id:+ ($new_id)}"
}

# ── upload_batch ─────────────────────────────────────────────────────

upload_batch() {
    local name="$1"; shift

    # --only filter.
    if [ -n "$ONLY" ] && [[ "$name" != *"$ONLY"* ]]; then return; fi

    # --file: skip bundles that don't contain the target file.
    local _file_force_saved=""
    if [ -n "$TARGET_FILE" ]; then
        local _found=false f abs_f
        for f in "$@"; do
            abs_f=$(cd "$(dirname "$f")" 2>/dev/null && echo "$(pwd)/$(basename "$f")")
            if [ "$abs_f" = "$TARGET_FILE" ]; then
                _found=true
                break
            fi
        done
        if ! $_found; then return; fi
        echo "  MATCH $name (contains $(basename "$TARGET_FILE"))"
        _file_force_saved=$FORCE
        FORCE=true
    fi

    # --list mode.
    if $LIST_ONLY; then
        local found=false pt
        if source_exists "$name"; then
            echo "  [EXISTS] $name ($(source_id_for "$name"))"
            found=true
        fi
        for pt in $(seq 2 "$NLM_SYNC_MAX_PARTS"); do
            if source_exists "$name (pt$pt)"; then
                echo "  [EXISTS] $name (pt$pt) ($(source_id_for "$name (pt$pt)"))"
                found=true
            fi
        done
        $found || echo "  [MISSING] $name"
        [ -n "$_file_force_saved" ] && FORCE=$_file_force_saved
        return
    fi

    # Collect files (guard empty array for set -u).
    local files=()
    if [ $# -gt 0 ]; then
        files=("$@")
    fi
    if [ ${#files[@]} -eq 0 ]; then
        echo "  SKIP $name (no files)"
        [ -n "$_file_force_saved" ] && FORCE=$_file_force_saved
        return
    fi

    # Check if txtar works on these files.
    local txtar_ok=true
    echo | txtar "${files[0]}" >/dev/null 2>&1 || txtar_ok=false

    # Split into chunks under NLM_SYNC_MAX_BYTES.
    local part=1 chunk_files=() chunk_size=0
    local f fsize overhead entry_size
    for f in "${files[@]}"; do
        fsize=$(wc -c < "$f" 2>/dev/null | tr -d ' ')
        overhead=$(( ${#f} + 20 ))
        entry_size=$(( fsize + overhead ))
        if [ ${#chunk_files[@]} -gt 0 ] && [ $(( chunk_size + entry_size )) -gt "$NLM_SYNC_MAX_BYTES" ]; then
            _upload_chunk "$name" "$part" "$txtar_ok" "${chunk_files[@]}"
            part=$(( part + 1 )); chunk_files=(); chunk_size=0
        fi
        chunk_files+=("$f"); chunk_size=$(( chunk_size + entry_size ))
    done
    if [ ${#chunk_files[@]} -gt 0 ]; then
        _upload_chunk "$name" "$part" "$txtar_ok" "${chunk_files[@]}"
    fi

    # Restore FORCE if overridden by --file match.
    [ -n "$_file_force_saved" ] && FORCE=$_file_force_saved
}

# ── File finders ─────────────────────────────────────────────────────

find_go() {
    local dir="$1"
    find "$dir" -type f \( -name '*.go' -o -name '*.md' \) \
        -not -path '*/.git/*' \
        -not -path '*/.gomodcache*' \
        -not -path '*/vendor/*' \
        -not -path '*/testdata/*' \
        -not -path '*/.build/*' \
        2>/dev/null | sort
}

find_go_only() {
    local dir="$1"
    find "$dir" -type f -name '*.go' \
        -not -path '*/.git/*' \
        -not -path '*/.gomodcache*' \
        -not -path '*/vendor/*' \
        -not -path '*/testdata/*' \
        2>/dev/null | sort
}

find_py() {
    local dir="$1"
    find "$dir" -type f \( -name '*.py' -o -name '*.md' \) \
        -not -path '*/.git/*' \
        -not -path '*/__pycache__/*' \
        -not -path '*/venv/*' \
        -not -path '*/.venv/*' \
        -not -path '*/build/*' \
        -not -path '*/dist/*' \
        -not -path '*/*.egg-info/*' \
        -not -path '*/.eggs/*' \
        -not -path '*/node_modules/*' \
        2>/dev/null | sort
}

find_swift() {
    local dir="$1"
    find "$dir" -type f -name '*.swift' \
        -not -path '*/.git/*' \
        -not -path '*/.build/*' \
        -not -path '*/Tests/*' \
        2>/dev/null | sort
}

find_src() {
    local dir="$1"
    find "$dir" -type f \( -name '*.go' -o -name '*.md' -o -name '*.proto' -o -name '*.metal' \) \
        -not -path '*/.git/*' \
        -not -path '*/.gomodcache*' \
        -not -path '*/.gocache*' \
        -not -path '*/.claude/*' \
        -not -path '*/.beads/*' \
        -not -path '*/vendor/*' \
        -not -path '*/testdata/*' \
        -not -path '*/.build/*' \
        -not -name '*.sum' \
        -not -name '*.bak*' \
        2>/dev/null | sort
}

find_src_broad() {
    local dir="$1"
    find "$dir" -type f \
        \( \
            -name '*.go' -o -name '*.rs' -o -name '*.toml' -o -name '*.md' -o \
            -name '*.sh' -o -name '*.py' -o -name '*.proto' -o \
            -name '*.ts' -o -name '*.tsx' -o -name '*.js' -o \
            -name '*.c' -o -name '*.cc' -o -name '*.cpp' -o \
            -name '*.h' -o -name '*.hpp' -o -name '*.yml' -o -name '*.yaml' -o \
            -name '*.json' -o -name 'Cargo.toml' -o -name 'go.mod' -o \
            -name 'Makefile' \
        \) \
        -not -path '*/.git/*' \
        -not -path '*/node_modules/*' \
        -not -path '*/target/*' \
        -not -path '*/dist/*' \
        -not -path '*/build/*' \
        -not -path '*/vendor/*' \
        -not -path '*/.venv/*' \
        -not -path '*/.claude/*' \
        -not -path '*/.beads/*' \
        -not -name '*.sum' \
        -not -name '*.png' -not -name '*.jpg' -not -name '*.gif' \
        -not -name '*.so' -not -name '*.dylib' -not -name '*.a' \
        2>/dev/null | sort
}

# ── Convenience uploaders ────────────────────────────────────────────

# upload_dirs "name" dir1 dir2 ...
# Uses find_go by default.  Override with NLM_SYNC_FINDER=find_py.
upload_dirs() {
    local name="$1"; shift
    local finder="${NLM_SYNC_FINDER:-find_go}"
    local -a files=()
    local dir
    for dir in "$@"; do
        [ -d "$dir" ] || continue
        while IFS= read -r f; do files+=("$f"); done < <("$finder" "$dir")
    done
    if [ ${#files[@]} -gt 0 ]; then
        upload_batch "$name" "${files[@]}"
    else
        upload_batch "$name"
    fi
}

# upload_py_dirs "name" dir1 dir2 ...
upload_py_dirs() {
    NLM_SYNC_FINDER=find_py upload_dirs "$@"
}

# upload_swift_dirs "name" dir1 dir2 ...
upload_swift_dirs() {
    NLM_SYNC_FINDER=find_swift upload_dirs "$@"
}

# upload_files "name" file1 file2 ...
# Only includes files that exist.
upload_files() {
    local name="$1"; shift
    local -a files=()
    local f
    for f in "$@"; do
        [ -f "$f" ] && files+=("$f")
    done
    if [ ${#files[@]} -gt 0 ]; then
        upload_batch "$name" "${files[@]}"
    else
        upload_batch "$name"
    fi
}

# upload_collected "name" < <(find ...)
# Reads file paths from stdin.
upload_collected() {
    local name="$1"
    local -a files=()
    local f
    while IFS= read -r f; do
        [ -n "$f" ] && files+=("$f")
    done
    if [ ${#files[@]} -gt 0 ]; then
        upload_batch "$name" "${files[@]}"
    else
        upload_batch "$name"
    fi
}

# upload_text "name" /path/to/textfile
# Uploads raw text (not txtar).  Supports content hashing.
upload_text() {
    local name="$1" textfile="$2"

    if [ -n "$ONLY" ] && [[ "$name" != *"$ONLY"* ]]; then return; fi

    local content_hash
    content_hash=$(shasum -a 256 < "$textfile" | cut -c1-64)

    local old_id=""
    if source_exists "$name"; then
        if ! $FORCE; then
            local prev_hash
            prev_hash=$(_nlm_cached_hash "$name")
            if [ -n "$prev_hash" ] && [ "$content_hash" = "$prev_hash" ]; then
                echo "  SKIP $name (unchanged)"
                return
            fi
            if [ -z "$prev_hash" ]; then
                echo "  SKIP $name (exists, no hash — use --force)"
                return
            fi
            echo "  CHANGED $name (hash differs)"
        fi
        old_id=$(source_id_for "$name")
    fi

    local text_size
    text_size=$(wc -c < "$textfile" | tr -d ' ')

    if $DRY_RUN; then
        echo "  WOULD UPLOAD $name (${text_size} bytes)"
        return
    fi

    echo "  UPLOAD $name (${text_size} bytes)"
    local new_id=""

    if [ -n "$old_id" ]; then
        new_id=$(nlm add "$NOTEBOOK_ID" - --name "$name" --replace "$old_id" < "$textfile" 2> >(_nlm_filter_noise >&2)) || {
            echo "  ERROR uploading $name" >&2
            return 1
        }
    else
        new_id=$(nlm add "$NOTEBOOK_ID" - --name "$name" < "$textfile" 2> >(_nlm_filter_noise >&2)) || {
            echo "  ERROR uploading $name" >&2
            return 1
        }
    fi

    if [ -n "$NLM_SYNC_DELAY" ] && [ "$NLM_SYNC_DELAY" -gt 0 ] 2>/dev/null; then
        sleep "$NLM_SYNC_DELAY"
    fi

    _nlm_save_hash "$name" "$content_hash"
    echo "  OK $name"
}

# upload_text_chunked "name" /path/to/textfile
# Splits at heading boundaries (## or #) when over NLM_SYNC_MAX_BYTES.
upload_text_chunked() {
    local base_name="$1" textfile="$2"
    local text_size
    text_size=$(wc -c < "$textfile" | tr -d ' ')

    if [ "$text_size" -le "$NLM_SYNC_MAX_BYTES" ]; then
        upload_text "$base_name" "$textfile"
        return
    fi

    local part=1
    local chunk_file
    chunk_file=$(mktemp)
    local chunk_size=0

    while IFS= read -r line; do
        local line_size=$(( ${#line} + 1 ))
        if [ "$chunk_size" -gt 0 ] && [ $(( chunk_size + line_size )) -gt "$NLM_SYNC_MAX_BYTES" ] && [[ "$line" == "## "* || "$line" == "# "* ]]; then
            local part_name="$base_name"
            [ "$part" -gt 1 ] && part_name="$base_name (pt$part)"
            upload_text "$part_name" "$chunk_file"
            part=$(( part + 1 ))
            : > "$chunk_file"
            chunk_size=0
        fi
        echo "$line" >> "$chunk_file"
        chunk_size=$(( chunk_size + line_size ))
    done < "$textfile"

    if [ "$chunk_size" -gt 0 ]; then
        local part_name="$base_name"
        [ "$part" -gt 1 ] && part_name="$base_name (pt$part)"
        upload_text "$part_name" "$chunk_file"
    fi

    rm -f "$chunk_file"
}
