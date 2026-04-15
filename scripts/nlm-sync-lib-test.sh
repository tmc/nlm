#!/bin/bash
# nlm-sync-lib-test.sh — Test harness for nlm-sync-lib.sh.
#
# Validates library functions using a mock nlm command.
# Run: bash nlm-sync-lib-test.sh

set -uo pipefail

TEST_DIR=$(mktemp -d)
PASS=0
FAIL=0

cleanup() { rm -rf "$TEST_DIR"; }
trap cleanup EXIT

# ── Test infrastructure ──────────────────────────────────────────────

assert_eq() {
    local label="$1" expected="$2" actual="$3"
    if [ "$expected" = "$actual" ]; then
        PASS=$(( PASS + 1 ))
    else
        FAIL=$(( FAIL + 1 ))
        echo "FAIL: $label"
        echo "  expected: $(printf '%q' "$expected")"
        echo "  actual:   $(printf '%q' "$actual")"
    fi
}

assert_contains() {
    local label="$1" haystack="$2" needle="$3"
    if echo "$haystack" | grep -qF "$needle"; then
        PASS=$(( PASS + 1 ))
    else
        FAIL=$(( FAIL + 1 ))
        echo "FAIL: $label"
        echo "  expected to contain: $needle"
        echo "  in: $haystack"
    fi
}

assert_not_contains() {
    local label="$1" haystack="$2" needle="$3"
    if ! echo "$haystack" | grep -qF "$needle"; then
        PASS=$(( PASS + 1 ))
    else
        FAIL=$(( FAIL + 1 ))
        echo "FAIL: $label"
        echo "  expected NOT to contain: $needle"
        echo "  in: $haystack"
    fi
}

assert_exit_ok() {
    local label="$1"; shift
    if "$@" >/dev/null 2>&1; then
        PASS=$(( PASS + 1 ))
    else
        FAIL=$(( FAIL + 1 ))
        echo "FAIL: $label (exit code $?)"
    fi
}

assert_exit_fail() {
    local label="$1"; shift
    if ! "$@" >/dev/null 2>&1; then
        PASS=$(( PASS + 1 ))
    else
        FAIL=$(( FAIL + 1 ))
        echo "FAIL: $label (expected failure, got success)"
    fi
}

# ── Mock nlm ─────────────────────────────────────────────────────────

MOCK_SOURCES_FILE="$TEST_DIR/mock_sources.txt"
MOCK_NLM_LOG="$TEST_DIR/nlm_calls.log"
MOCK_NLM_ADD_ID="aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"

cat > "$TEST_DIR/nlm" <<'MOCK'
#!/bin/bash
LOG="${MOCK_NLM_LOG:-/dev/null}"
echo "$*" >> "$LOG"
case "$1" in
    sources)
        if [ "${2:-}" = "--json" ] || [ "${3:-}" = "--json" ]; then
            cat "${MOCK_SOURCES_JSON:-/dev/null}" 2>/dev/null
        else
            cat "${MOCK_SOURCES_FILE:-/dev/null}" 2>/dev/null
        fi
        ;;
    add)
        echo "$MOCK_NLM_ADD_ID" >&1
        ;;
    rename-source|rm-source)
        ;;
    --yes)
        # nlm --yes rm-source ...
        ;;
esac
MOCK
chmod +x "$TEST_DIR/nlm"

# Also mock txtar (some systems may not have it).
cat > "$TEST_DIR/txtar" <<'MOCK'
#!/bin/bash
# Minimal txtar mock: cat files with headers.
echo ""  # comment line
for f in "$@"; do
    if [ -f "$f" ]; then
        echo "-- $f --"
        cat "$f"
    fi
done
MOCK
chmod +x "$TEST_DIR/txtar"

# Put mocks on PATH.
export PATH="$TEST_DIR:$PATH"
MOCK_SOURCES_JSON="$TEST_DIR/mock_sources.json"
export MOCK_SOURCES_FILE MOCK_SOURCES_JSON MOCK_NLM_LOG MOCK_NLM_ADD_ID

# ── Source the library ───────────────────────────────────────────────

# The lib checks for deps on source; our mocks satisfy nlm and txtar-c is real.
# Create a txtar-c mock too if not available.
if ! command -v txtar-c >/dev/null 2>&1; then
    cat > "$TEST_DIR/txtar-c" <<'MOCK'
#!/bin/bash
echo "-- mock --"
echo "content"
MOCK
    chmod +x "$TEST_DIR/txtar-c"
fi

# Create shasum mock if not available.
if ! command -v shasum >/dev/null 2>&1; then
    if command -v sha256sum >/dev/null 2>&1; then
        cat > "$TEST_DIR/shasum" <<'MOCK'
#!/bin/bash
sha256sum "$@" | sed 's/  /  /'
MOCK
        chmod +x "$TEST_DIR/shasum"
    fi
fi

source "$(dirname "$0")/nlm-sync-lib.sh"

# ══════════════════════════════════════════════════════════════════════
# Tests
# ══════════════════════════════════════════════════════════════════════

echo "Running nlm-sync-lib tests..."
echo ""

# ── Test: parse_args sets NOTEBOOK_ID ────────────────────────────────

NOTEBOOK_ID="" FORCE=false ONLY="" LIST_ONLY=false DRY_RUN=false TARGET_FILE=""
nlm_sync_parse_args "abc123"
assert_eq "parse_args: notebook ID" "abc123" "$NOTEBOOK_ID"
assert_eq "parse_args: force default" "false" "$FORCE"

# ── Test: parse_args sets flags ──────────────────────────────────────

NOTEBOOK_ID="" FORCE=false ONLY="" LIST_ONLY=false DRY_RUN=false TARGET_FILE=""
nlm_sync_parse_args "nb1" --force --only "core" --dry-run
assert_eq "parse_args: force" "true" "$FORCE"
assert_eq "parse_args: only" "core" "$ONLY"
assert_eq "parse_args: dry-run" "true" "$DRY_RUN"
assert_eq "parse_args: notebook" "nb1" "$NOTEBOOK_ID"

# ── Test: parse_args --list ──────────────────────────────────────────

NOTEBOOK_ID="" FORCE=false ONLY="" LIST_ONLY=false DRY_RUN=false TARGET_FILE=""
nlm_sync_parse_args "nb2" --list
assert_eq "parse_args: list" "true" "$LIST_ONLY"

# ── Test: parse_args fails without notebook ID ───────────────────────

NOTEBOOK_ID="" FORCE=false ONLY="" LIST_ONLY=false DRY_RUN=false TARGET_FILE=""
assert_exit_fail "parse_args: no notebook ID" nlm_sync_parse_args

# ── Test: content hashing ────────────────────────────────────────────

NOTEBOOK_ID="test-nb-hash"
_nlm_hash_init

_nlm_save_hash "test-source" "abc123hash"
result=$(_nlm_cached_hash "test-source")
assert_eq "hash: save and retrieve" "abc123hash" "$result"

# Content changed detection.
_nlm_content_changed "test-source" "different-hash"
assert_eq "hash: changed returns 0" "0" "$?"

_nlm_content_changed "test-source" "abc123hash"
rc=$?
assert_eq "hash: unchanged returns 1" "1" "$rc"

# No cached hash.
_nlm_content_changed "never-seen-source" "anything"
assert_eq "hash: no cache returns 0" "0" "$?"

# ── Test: source cache ───────────────────────────────────────────────

NOTEBOOK_ID="test-nb-cache"
cat > "$MOCK_SOURCES_JSON" <<'EOF'
[
  {"source_id": "aaaaaaaa-1111-2222-3333-444444444444", "title": "my-source", "source_type": "text", "status": "ready", "last_modified": "2026-04-15"},
  {"source_id": "bbbbbbbb-1111-2222-3333-444444444444", "title": "other-source (pt2)", "source_type": "text", "status": "ready", "last_modified": "2026-04-15"}
]
EOF
refresh_sources_cache

source_exists "my-source"
assert_eq "cache: source exists" "0" "$?"

! source_exists "nonexistent"
assert_eq "cache: source not exists" "0" "$?"

sid=$(source_id_for "my-source")
assert_eq "cache: source_id_for" "aaaaaaaa-1111-2222-3333-444444444444" "$sid"

# Exact match: "my-source" should NOT match "my-source-extended"
! source_exists "my-sourc"
assert_eq "cache: no substring match" "0" "$?"

# ── Test: upload_batch with empty file list ──────────────────────────

NOTEBOOK_ID="test-nb-empty"
FORCE=false ONLY="" LIST_ONLY=false DRY_RUN=false TARGET_FILE=""
_nlm_hash_init
echo '[]' > "$MOCK_SOURCES_JSON"
refresh_sources_cache

output=$(upload_batch "empty-bundle" 2>&1)
assert_contains "upload_batch: empty skip" "$output" "SKIP empty-bundle (no files)"

# ── Test: upload_batch --only filter ─────────────────────────────────

ONLY="core"
output=$(upload_batch "unrelated-name" 2>&1)
# Should silently skip (no output).
assert_eq "upload_batch: only filter skips" "" "$output"
ONLY=""

# ── Test: upload_batch --list mode ───────────────────────────────────

NOTEBOOK_ID="test-nb-list"
cat > "$MOCK_SOURCES_JSON" <<'EOF'
[{"source_id": "aaaaaaaa-1111-2222-3333-444444444444", "title": "my-bundle", "source_type": "text", "status": "ready", "last_modified": "2026-04-15"}]
EOF
refresh_sources_cache
LIST_ONLY=true

output=$(upload_batch "my-bundle" 2>&1)
assert_contains "upload_batch: list exists" "$output" "[EXISTS] my-bundle"

output=$(upload_batch "missing-bundle" 2>&1)
assert_contains "upload_batch: list missing" "$output" "[MISSING] missing-bundle"

LIST_ONLY=false

# ── Test: upload_batch --dry-run ─────────────────────────────────────

NOTEBOOK_ID="test-nb-dryrun"
echo '[]' > "$MOCK_SOURCES_JSON"
refresh_sources_cache
DRY_RUN=true
_nlm_hash_init

# Create a test file.
echo "hello world" > "$TEST_DIR/testfile.txt"

: > "$MOCK_NLM_LOG"
output=$(upload_batch "dry-bundle" "$TEST_DIR/testfile.txt" 2>&1)
assert_contains "upload_batch: dry-run output" "$output" "WOULD UPLOAD dry-bundle"

# Verify no nlm add was called.
if [ -f "$MOCK_NLM_LOG" ]; then
    nlm_calls=$(cat "$MOCK_NLM_LOG")
    assert_not_contains "upload_batch: dry-run no add" "$nlm_calls" "add"
fi

DRY_RUN=false

# ── Test: upload_batch chunks at MAX_BYTES ───────────────────────────

NOTEBOOK_ID="test-nb-chunk"
echo '[]' > "$MOCK_SOURCES_JSON"
refresh_sources_cache
DRY_RUN=true
_nlm_hash_init
NLM_SYNC_MAX_BYTES=100  # very small to force splitting

# Create two files that together exceed 100 bytes.
printf '%80s' " " > "$TEST_DIR/big1.txt"
printf '%80s' " " > "$TEST_DIR/big2.txt"

output=$(upload_batch "chunk-test" "$TEST_DIR/big1.txt" "$TEST_DIR/big2.txt" 2>&1)
# Should produce two parts.
assert_contains "upload_batch: chunk pt1" "$output" "WOULD UPLOAD chunk-test"
assert_contains "upload_batch: chunk pt2" "$output" "WOULD UPLOAD chunk-test (pt2)"

NLM_SYNC_MAX_BYTES=450000
DRY_RUN=false

# ── Test: upload_text ────────────────────────────────────────────────

NOTEBOOK_ID="test-nb-text"
echo '[]' > "$MOCK_SOURCES_JSON"
refresh_sources_cache
DRY_RUN=true
_nlm_hash_init

echo "some text content" > "$TEST_DIR/text_input.txt"
output=$(upload_text "text-source" "$TEST_DIR/text_input.txt" 2>&1)
assert_contains "upload_text: dry-run" "$output" "WOULD UPLOAD text-source"

DRY_RUN=false

# ── Test: upload_text content hash skip ──────────────────────────────

NOTEBOOK_ID="test-nb-hashskip"
_nlm_hash_init

# Simulate existing source + matching hash.
echo "cached content" > "$TEST_DIR/cached.txt"
hash_val=$(shasum -a 256 < "$TEST_DIR/cached.txt" | cut -c1-64)
_nlm_save_hash "cached-source" "$hash_val"
cat > "$MOCK_SOURCES_JSON" <<'EOF'
[{"source_id": "aaaaaaaa-1111-2222-3333-444444444444", "title": "cached-source", "source_type": "text", "status": "ready", "last_modified": "2026-04-15"}]
EOF
refresh_sources_cache

output=$(upload_text "cached-source" "$TEST_DIR/cached.txt" 2>&1)
assert_contains "upload_text: hash skip" "$output" "SKIP cached-source (unchanged)"

# ── Test: upload_text content hash detects change ────────────────────

echo "different content now" > "$TEST_DIR/cached.txt"
DRY_RUN=true
output=$(upload_text "cached-source" "$TEST_DIR/cached.txt" 2>&1)
assert_contains "upload_text: hash change" "$output" "CHANGED cached-source"
DRY_RUN=false

# ── Test: find_go ────────────────────────────────────────────────────

mkdir -p "$TEST_DIR/repo/pkg" "$TEST_DIR/repo/.git" "$TEST_DIR/repo/vendor"
echo "package pkg" > "$TEST_DIR/repo/pkg/main.go"
echo "# readme" > "$TEST_DIR/repo/pkg/README.md"
echo "vendor" > "$TEST_DIR/repo/vendor/v.go"
echo "git" > "$TEST_DIR/repo/.git/config"

found=$(find_go "$TEST_DIR/repo" | tr '\n' ' ')
assert_contains "find_go: includes .go" "$found" "main.go"
assert_contains "find_go: includes .md" "$found" "README.md"
assert_not_contains "find_go: excludes vendor" "$found" "vendor"
assert_not_contains "find_go: excludes .git" "$found" ".git/config"

# ── Test: find_py ────────────────────────────────────────────────────

mkdir -p "$TEST_DIR/pyrepo/src" "$TEST_DIR/pyrepo/__pycache__" "$TEST_DIR/pyrepo/.venv"
echo "print(1)" > "$TEST_DIR/pyrepo/src/main.py"
echo "cache" > "$TEST_DIR/pyrepo/__pycache__/x.py"
echo "venv" > "$TEST_DIR/pyrepo/.venv/y.py"

found=$(find_py "$TEST_DIR/pyrepo" | tr '\n' ' ')
assert_contains "find_py: includes .py" "$found" "main.py"
assert_not_contains "find_py: excludes __pycache__" "$found" "__pycache__"
assert_not_contains "find_py: excludes .venv" "$found" ".venv"

# ── Test: _nlm_filter_noise ─────────────────────────────────────────

noisy=$(printf 'good line\ninvalid character blah\nDEBUG something\nAre you sure\nanother good line\n')
filtered=$(echo "$noisy" | _nlm_filter_noise)
assert_contains "filter: keeps good lines" "$filtered" "good line"
assert_contains "filter: keeps another" "$filtered" "another good line"
assert_not_contains "filter: removes invalid character" "$filtered" "invalid character"
assert_not_contains "filter: removes DEBUG" "$filtered" "DEBUG"
assert_not_contains "filter: removes Are you" "$filtered" "Are you"

# ── Test: nlm_sync_require_dir ───────────────────────────────────────

assert_exit_ok "require_dir: exists" nlm_sync_require_dir "$TEST_DIR"
# Can't easily test exit 1 without subprocess.

# ══════════════════════════════════════════════════════════════════════
# Results
# ══════════════════════════════════════════════════════════════════════

echo ""
echo "═══════════════════════════════════════════════════"
echo "  Results: $PASS passed, $FAIL failed"
echo "═══════════════════════════════════════════════════"

if [ "$FAIL" -gt 0 ]; then
    exit 1
fi
