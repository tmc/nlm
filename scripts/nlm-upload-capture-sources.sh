#!/bin/bash
# nlm-upload-capture-sources.sh — Upload captured NotebookLM source files.
#
# Uploads files from the chrome-to-har capture tree into a NotebookLM notebook.
# Each file gets a stable source name. If an upload fails, the failed blob is
# split into smaller byte ranges and retried recursively until it uploads or can
# no longer be split.
#
# Usage:
#   nlm-upload-capture-sources.sh <notebook-id>
#   nlm-upload-capture-sources.sh <notebook-id> --dry-run
#   nlm-upload-capture-sources.sh <notebook-id> --force
#   nlm-upload-capture-sources.sh <notebook-id> --only "www.gstatic.com"
#
# Requirements: nlm, bash 3.2+, shasum, tail, head

set -uo pipefail

SOURCE_ROOT="${SOURCE_ROOT:-$HOME/go/src/github.com/tmc/misc/chrome-to-har/logs/nlm-capture/sources}"
PREFIX="${PREFIX:-nlm-capture}"
MAX_BYTES="${MAX_BYTES:-425000}"
MIN_SPLIT_BYTES="${MIN_SPLIT_BYTES:-1024}"
NOTEBOOK_ID=""
DRY_RUN=false
FORCE=false
ONLY=""

sources_cache=""
uploaded_count=0
skipped_count=0
failed_count=0

usage() {
    sed -n '2,/^$/s/^# \{0,1\}//p' "$0"
}

die() {
    echo "error: $*" >&2
    exit 1
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --dry-run) DRY_RUN=true; shift ;;
        --force)   FORCE=true; shift ;;
        --only)    ONLY="$2"; shift 2 ;;
        --max-bytes) MAX_BYTES="$2"; shift 2 ;;
        --source-root) SOURCE_ROOT="$2"; shift 2 ;;
        --prefix) PREFIX="$2"; shift 2 ;;
        --help|-h) usage; exit 0 ;;
        *)         NOTEBOOK_ID="$1"; shift ;;
    esac
done

[ -n "$NOTEBOOK_ID" ] || die "usage: $(basename "$0") <notebook-id> [--dry-run] [--force] [--only FILTER]"
[ -d "$SOURCE_ROOT" ] || die "SOURCE_ROOT=$SOURCE_ROOT does not exist"
command -v nlm >/dev/null 2>&1 || die "nlm is not installed"
command -v shasum >/dev/null 2>&1 || die "shasum is not installed"

refresh_sources_cache() {
    if $DRY_RUN; then
        sources_cache=""
        return
    fi
    sources_cache=$(nlm sources "$NOTEBOOK_ID" 2>&1 || true)
}

source_id_for_exact() {
    source_ids_for_exact "$1" | head -n 1
}

source_ids_for_exact() {
    local title="$1"
    printf '%s\n' "$sources_cache" | awk -v target="$title" '
        NR == 1 { next }
        {
            id = $1
            $1 = ""
            sub(/^ +/, "", $0)
            sub(/ +SOURCE_TYPE_[^ ]+ +SOURCE_STATUS_[^ ]+ +[0-9TZ:-]+$/, "", $0)
            if ($0 == target) {
                print id
            }
        }
    '
}

source_exists() {
    [ -n "$(source_id_for_exact "$1")" ]
}

sanitize_token() {
    printf '%s' "$1" | tr '[:upper:]' '[:lower:]' | sed 's/[^a-z0-9._-]/-/g; s/-\{2,\}/-/g; s/^-//; s/-$//'
}

label_token_for_relpath() {
    local rel="$1"
    case "$rel" in
        *boq-labs-tailwind*) printf '%s' "boq-labs-tailwind" ;;
        *boq-one-google*)    printf '%s' "boq-one-google" ;;
        *googleapis_client*) printf '%s' "googleapis-client" ;;
        */_compiled/og/_/js/*) printf '%s' "og-js" ;;
        */_compiled/og/_/ss/*) printf '%s' "og-css" ;;
        */widget/account)    printf '%s' "widget-account" ;;
        */widget/app)        printf '%s' "widget-app" ;;
        */gtm.js)            printf '%s' "gtm.js" ;;
        */css2)              printf '%s' "css2" ;;
        */index)             printf '%s' "index" ;;
        */notebook/*)        printf '%s' "notebook-page" ;;
        *)                   sanitize_token "$(basename "$rel")" ;;
    esac
}

base_label_for_file() {
    local rel="$1"
    local host="${rel%%/*}"
    local token
    local hash

    token=$(label_token_for_relpath "$rel")
    [ -n "$token" ] || token="blob"
    hash=$(printf '%s' "$rel" | shasum | awk '{print substr($1,1,8)}')
    printf '%s %s %s [%s]' "$PREFIX" "$host" "$token" "$hash"
}

format_label() {
    local base="$1"
    local suffix="$2"
    if [ -z "$suffix" ]; then
        printf '%s' "$base"
        return
    fi
    printf '%s (pt%s)' "$base" "$suffix"
}

file_size() {
    wc -c < "$1" | tr -d ' '
}

upload_full_file() {
    local label="$1"
    local path="$2"
    nlm --name "$label" add "$NOTEBOOK_ID" "$path"
}

upload_slice_from_file() {
    local label="$1"
    local path="$2"
    local start="$3"
    local length="$4"
    local tmp
    local tmpdir="${TMPDIR:-/tmp}"
    tmpdir="${tmpdir%/}"
    tmp=$(mktemp "$tmpdir/nlm-capture-slice.XXXXXX")
    dd if="$path" of="$tmp" bs=1 skip="$start" count="$length" 2>/dev/null
    nlm --name "$label" --mime text/plain add "$NOTEBOOK_ID" "$tmp"
    local status=$?
    rm -f "$tmp"
    return "$status"
}

remove_old_source() {
    local source_id="$1"
    [ -n "$source_id" ] || return 0
    echo "  REMOVE old $source_id"
    nlm --yes rm-source "$NOTEBOOK_ID" "$source_id" >/dev/null 2>&1 || true
}

remove_failed_sources_for_label() {
    local label="$1"
    local keep_id="$2"
    local found=0
    local source_id

    while IFS= read -r source_id; do
        [ -n "$source_id" ] || continue
        if [ -n "$keep_id" ] && [ "$source_id" = "$keep_id" ]; then
            continue
        fi
        echo "  REMOVE failed $source_id"
        nlm --yes rm-source "$NOTEBOOK_ID" "$source_id" >/dev/null 2>&1 || true
        found=1
    done <<EOF
$(source_ids_for_exact "$label")
EOF

    if [ "$found" -eq 1 ]; then
        refresh_sources_cache
    fi
}

try_upload_piece() {
    local label="$1"
    local path="$2"
    local start="$3"
    local length="$4"
    local total_size="$5"
    local old_id="$6"
    local output=""
    local status=0

    if $DRY_RUN; then
        if [ "$start" -eq 0 ] && [ "$length" -eq "$total_size" ]; then
            echo "  WOULD UPLOAD $label ($length bytes, file)"
        else
            echo "  WOULD UPLOAD $label ($length bytes, slice $start+$length)"
        fi
        uploaded_count=$(( uploaded_count + 1 ))
        return 0
    fi

    if [ "$start" -eq 0 ] && [ "$length" -eq "$total_size" ]; then
        echo "  UPLOAD $label ($length bytes, file)"
        output=$(upload_full_file "$label" "$path" 2>&1)
        status=$?
    else
        echo "  UPLOAD $label ($length bytes, slice $start+$length)"
        output=$(upload_slice_from_file "$label" "$path" "$start" "$length" 2>&1)
        status=$?
    fi

    if [ -n "$output" ]; then
        printf '%s\n' "$output" | grep -v '^Reading from stdin\.\.\.$' || true
    fi

    if [ "$status" -eq 0 ]; then
        refresh_sources_cache
        remove_old_source "$old_id"
        refresh_sources_cache
        uploaded_count=$(( uploaded_count + 1 ))
        echo "  OK $label"
        return 0
    fi

    refresh_sources_cache
    remove_failed_sources_for_label "$label" "$old_id"

    return 1
}

upload_piece_recursive() {
    local base="$1"
    local suffix="$2"
    local path="$3"
    local start="$4"
    local length="$5"
    local total_size="$6"
    local label
    local old_id=""
    local left_suffix=""
    local right_suffix=""
    local left_len=0
    local right_len=0

    label=$(format_label "$base" "$suffix")

    if [ -n "$ONLY" ] && [[ "$label" != *"$ONLY"* ]]; then
        return 0
    fi

    if source_exists "$label" && ! $FORCE; then
        echo "  SKIP $label (exists)"
        skipped_count=$(( skipped_count + 1 ))
        return 0
    fi

    if $FORCE; then
        old_id=$(source_id_for_exact "$label")
    fi

    if [ "$length" -gt "$MAX_BYTES" ]; then
        echo "  SPLIT $label ($length bytes > $MAX_BYTES)"
    else
        if try_upload_piece "$label" "$path" "$start" "$length" "$total_size" "$old_id"; then
            return 0
        fi
        if [ "$length" -le "$MIN_SPLIT_BYTES" ]; then
            echo "  FAIL $label (cannot split below $MIN_SPLIT_BYTES bytes)" >&2
            failed_count=$(( failed_count + 1 ))
            return 1
        fi
        echo "  RETRY AS SPLIT $label"
    fi

    left_len=$(( length / 2 ))
    right_len=$(( length - left_len ))
    [ "$left_len" -gt 0 ] || left_len=1
    right_len=$(( length - left_len ))

    left_suffix="1"
    right_suffix="2"
    if [ -n "$suffix" ]; then
        left_suffix="$suffix-1"
        right_suffix="$suffix-2"
    fi

    upload_piece_recursive "$base" "$left_suffix" "$path" "$start" "$left_len" "$total_size" || return 1
    upload_piece_recursive "$base" "$right_suffix" "$path" "$(( start + left_len ))" "$right_len" "$total_size" || return 1
}

upload_file() {
    local path="$1"
    local rel="${path#$SOURCE_ROOT/}"
    local size
    local base

    size=$(file_size "$path")
    base=$(base_label_for_file "$rel")

    echo
    echo "FILE $rel"
    echo "  base: $base"
    upload_piece_recursive "$base" "" "$path" 0 "$size" "$size"
}

main() {
    local files=()
    local f=""

    refresh_sources_cache

    while IFS= read -r f; do
        [ -n "$f" ] && files+=("$f")
    done < <(find "$SOURCE_ROOT" -type f | sort)

    echo "═══════════════════════════════════════════════════"
    echo "  nlm-upload-capture-sources: $NOTEBOOK_ID"
    echo "  root=$SOURCE_ROOT  max_bytes=$MAX_BYTES  force=$FORCE"
    echo "═══════════════════════════════════════════════════"

    if [ "${#files[@]}" -eq 0 ]; then
        echo
        echo "No files found under $SOURCE_ROOT"
        exit 0
    fi

    for f in "${files[@]}"; do
        upload_file "$f" || exit 1
    done

    echo
    echo "uploaded=$uploaded_count skipped=$skipped_count failed=$failed_count"
}

main "$@"
