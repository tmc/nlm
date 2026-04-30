---
title: Test Conventions
date: 2026-04-16
---

## Summary

Three rules for tests in this repo, each load-bearing enough that violating it has bitten us before:

1. **Check-in small golden fixtures under `testdata/`.** Read them directly — no skip guard.
2. **Never commit HAR / JSONL captures.** They go under `docs/captures/`, which is gitignored. Tests that depend on them **must** skip when the file is absent.
3. **Hand-written wire encoders need a HAR-citation guard comment.** An encoder without one is indistinguishable from a lucky guess.

Details below.

## Fixture taxonomy

Two kinds of test inputs live in this repo. They are not interchangeable.

### `testdata/` — checked in, read directly

Small, sanitized, version-controlled. Examples:

- `internal/method/testdata/r7cb6c_{audio,video,slides}_request.json` — golden-payload request bodies for the R7cb6c encoder.
- `internal/method/testdata/rc3d8d_rename_artifact_request.json` — golden-payload for rename-artifact.
- `internal/notebooklm/api/testdata/AUrzMb_analytics_response.json` — captured wire response preserved as redesign evidence for the analytics response model.

Tests that read these do so unconditionally:

```go
want, err := os.ReadFile(filepath.Join("testdata", "r7cb6c_audio_request.json"))
if err != nil {
    t.Fatalf("read golden: %v", err)
}
```

No skip guard needed — the file is in the repo. If it's missing, that's a real problem worth a red.

### `docs/captures/` — gitignored, contain secrets

Raw HAR / JSONL captures carry auth cookies, session tokens, and personal notebook contents. They cannot be checked in. `.gitignore` excludes `docs/captures/` wholesale.

Tests that depend on a capture must not `t.Fatal` when the file is absent. They must `t.Skip` with a message pointing at the missing path and the tool that produces it. See below.

## The fixture-skip pattern

Convention established by commit 3269fa2:

```go
// skipIfFixtureMissing skips the test when path is absent. Capture
// fixtures under docs/captures/ are gitignored (they contain auth
// cookies and session tokens), so contributors without a local capture
// run cannot exercise these tests.
func skipIfFixtureMissing(t *testing.T, path string) {
    t.Helper()
    if _, err := os.Stat(path); os.IsNotExist(err) {
        t.Skipf("fixture absent at %s; capture it via <tool> to enable this test", path)
    }
}

func TestDecodeInteractiveAudioCapture(t *testing.T) {
    path := filepath.Join("..", "..", "docs", "captures", "interactive-audio", "webrtc-datachannel.jsonl")
    skipIfFixtureMissing(t, path)
    frames, err := DecodeCaptureFile(path)
    // ... assertions
}
```

The `t.Skipf` message must include:

- The full path the test looked for (so the reader knows what to populate).
- A hint at how to populate it (capture tool name, driver script, or HAR export procedure).

A `t.Skip` with no message is worse than a `t.Fatal` — the reader sees green but doesn't know why the assertions didn't run.

### When to use it

- The fixture lives under `docs/captures/` or any other path that `.gitignore` excludes.
- The test's assertions only have value against real captured wire output (not synthetic data a test could fabricate).

### When NOT to use it

- Fixtures under `testdata/` — if one is missing, that's a real bug, not an environment gap.
- Fixtures the test itself writes inside the run (e.g., round-trip tests that write then read). Those should `t.Fatal` on failure — the environment should always be able to create the file.

## HAR-citation guard comment for encoders

Hand-written wire encoders in `internal/method/` must carry a comment pointing at the HAR capture or the live-use evidence that proves the shape is correct. Without one, a future reader cannot distinguish a HAR-verified encoder from a lucky guess that happens to work against a lenient server.

Current style in the tree:

```go
// Wire format verified against HAR capture:
// docs/captures/phase1/V5N4be_delete.txt (2026-04-07).
// Options blob treated as opaque literal: two captured calls used
// byte-identical bytes; if future captures show variation, re-derive
// from the fixture cited above.
func EncodeDeleteArtifactArgs(...) { ... }
```

For encoders whose provenance is live-use success rather than a passive HAR (e.g., rename-artifact, where the CLI already worked and we captured the shape from the running code, not from DevTools), the guard comment says so explicitly:

```go
// Wire format provenance: live-use success in api/client.go as of
// <commit-hash>. No passive HAR exists — rpcids=rc3d8d was never
// captured in the 2026-04-07 or 2026-04-16 sessions. If behavior
// changes, re-capture via Chrome DevTools on the rename flow.
```

The rule: a reader looking at the encoder should be able to tell *how* the shape was verified. Don't leave that implicit.

## What to commit vs. capture locally

| Kind | Location | Committed? | Test guard? |
|---|---|---|---|
| Golden-payload requests (scrubbed) | `internal/method/testdata/` | yes | none — `t.Fatal` on absence |
| Captured responses (scrubbed, used as evidence) | `internal/notebooklm/api/testdata/` | yes | none — `t.Fatal` on absence |
| Raw HAR / JSONL captures with secrets | `docs/captures/` (gitignored) | **no** | `skipIfFixtureMissing` pattern |
| Encoder guard-comment references | in source, next to the function | n/a | n/a |

If you capture a raw HAR, extract the request/response you need, scrub it, and save the scrubbed copy under `testdata/`. Leave the raw HAR under `docs/captures/` for local reference but do not commit it.

## Known exemptions

- `cmd/nlm/auth_retry_test.go` and similar tests write files into `t.TempDir()` during the run and read them back. Those should `t.Fatal` on read failure — the environment is expected to be able to create the file.
- `internal/method/testdata/` goldens are committed and should `t.Fatal` on absence.

## Followups

- Add a CI check that greps `internal/method/` hand-written encoders for a guard comment; warn on absence.
- When the `otmP3b` / revise-artifact captures land, they'll go under `testdata/` as scrubbed goldens, following the same pattern as R7cb6c and rc3d8d.
