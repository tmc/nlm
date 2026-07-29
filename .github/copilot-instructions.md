# nlm — repository instructions for coding agents

**Trust these instructions.** Every command below was executed against this commit, including
in a fresh `git clone`, and the observed output is what is reported. Search the tree only when
something here is incomplete or demonstrably wrong.

## What this repo is

`nlm` is a Go command-line client **and** MCP server for Google NotebookLM. It speaks two
undocumented Google wire protocols — `batchexecute` (URL-encoded, positionally-indexed nested
JSON arrays keyed by short rpc IDs like `o4cbdc`) and a gRPC-Web streaming endpoint for chat —
and maps them onto protobuf messages through a custom positional-JSON codec
(`internal/beprotojson`). Much of the repo's mass is the ongoing migration from hand-written
positional parsers to generated proto bindings; `docs/nlm-proto-migration-ledger.md` tracks
that per RPC family and is the source of truth for what is switched vs. still legacy.

- Single Go module `github.com/tmc/nlm`, `go 1.25.0`. Verified with go1.26.3 darwin/arm64.
- ~471 tracked files, 336 `.go` files, ~108k lines (about half is generated protobuf code).
- Real runtime deps: chromedp (browser cookie extraction), protobuf,
  `modelcontextprotocol/go-sdk`, `rsc.io/script` (CLI tests), `golang.org/x/*`.
  Not gRPC — NotebookLM speaks batchexecute and gRPC-Web over plain `net/http`.
- CI checks command reachability with `deadcode` after compiling integration-tagged tests.
  The full local gate remains `go build ./... && go vet ./... && go test ./...`.

## Build and test — the exact sequence

Run from the repo root. No bootstrap, no codegen, no credentials, and no network beyond the
initial module download. Tests are hermetic: they use an isolated `HOME` under `t.TempDir()`
and never reach the network.

```bash
go build ./...        # ~2s warm, ~4s cold — clean
go test ./...         # ~17-19s — all packages pass
```

Optional extra confidence (verified green):

```bash
go test -race ./internal/...                 # ~16s
go test ./cmd/nlm -run TestCLICommands -v    # the txtar script-test suite alone
```

**Some tests are behind `//go:build integration`** (`notebooklm/{client,comprehensive}_record_test.go`).
They are excluded from a default `go test ./...`, so anything they alone use looks unused to
`go vet`, `staticcheck`, and `deadcode`. Before deleting any "unused" symbol, confirm with
`go test -tags integration -run XXXNONE ./...`, which compiles those files without running them.

Always run `go build ./...` before `go test ./...`: `cmd/nlm`'s `TestMain` shells out to
`go build -o nlm_test .` inside `cmd/nlm/`, so that directory must be writable and `go` must be
on `PATH`. `nlm_test` is gitignored — never commit it.

`go mod tidy` is currently a **no-op** (zero diff). If it wants to change `go.mod`/`go.sum`,
you added a dependency — reconsider first. Never hand-edit `go.sum`.

## ⚠️ `go vet ./...` fails at HEAD — pre-existing, not yours

```
gen/method/list_recently_viewed_projects_test.go:19:67: call of t.Fatalf copies lock value:
  ...ListRecentlyViewedProjectsRequest contains protoimpl.MessageState contains sync.Mutex
```

That hand-written test passes a proto message **by value** to `t.Fatalf("...%+v", req)`. It is
the only vet finding in the repo. Use the scoped form, which **passes cleanly**:

```bash
go vet ./cmd/... ./internal/... ./proto/...   # exit 0
```

Do not let this failure block you, and do not silently "fix" unrelated code around it. (The
one-line fix is `%+v`, `&req` — make it only if your change already touches that file.)

## Codegen — it works, and it is reproducible

Older notes saying codegen requires a Buf Schema Registry token are **obsolete**. Every tool
runs through `go run` at a pinned version and none of them is a module dependency: `buf`
v1.55.1 from the `go:generate` line in `proto/gen.go`, and `protoc-gen-go` v1.31.0 from
`proto/buf.gen.yaml`. No BSR credentials are needed. There is deliberately no `go-grpc`
plugin — see the comment in `buf.gen.yaml`.

```bash
go install github.com/tmc/misc/protoc-gen-anything@latest   # the one external prerequisite
go generate ./proto                                          # runs `go run .../buf@v1.55.1 generate`
git status --porcelain                                       # verified: zero drift
```

Verified to produce **zero diff**, so `gen/` is exactly in sync with `proto/`. The first run
builds buf from source and takes a few minutes; later runs hit the build cache. Without
`protoc-gen-anything` on `PATH` you get
`could not find protoc plugin for name anything`; install it, don't work around it.

## Where code goes (this trips people up)

- `gen/` is codegen output. Codegen writes only `*_encoder.go` (`gen/method/`) and
  `*_client.go` (`gen/service/`) plus `*.pb.go`. Anything you put in those filenames **will be
  clobbered**.
- `gen/method/` also holds 19 **hand-written** `*_test.go` files that codegen does not touch
  (confirmed by the zero-drift run). Tests for generated encoders belong there.
- **Hand-verified wire encoders go in `internal/method/`, never `gen/method/`.** Every file
  there must carry a provenance guard comment — `// Wire format verified against HAR capture:
  <path> — do not regenerate.`, or the explicit live-success / unverified variant. See
  `docs/dev/codegen.md`.
- Fixture policy (`docs/dev/test-conventions.md`): small scrubbed goldens live in `testdata/`
  and are read unconditionally (`t.Fatal` on absence). Raw HAR/JSONL captures live under
  `docs/captures/`, which is **gitignored because captures contain live auth cookies** — never
  commit one.

## Rules the test suite actually enforces

- **Adding a stable CLI command requires a docs edit.**
  `TestCommandReferenceCoversStableCommands` (`cmd/nlm/command_docs_test.go`) reads
  `docs/commands.md` and fails unless it contains the literal substring `` `nlm <name> `` for
  every non-hidden `surfaceStable` entry in the `commands` table. Reproduced: adding a command
  without the doc row fails with
  `docs/commands.md missing command table entries: [<name>]`. Commands marked `hidden: true`
  (e.g. `betool`) are exempt.
- **A command's `section` must be one of `helpSections`** (`cmd/nlm/commands.go`: Notebook,
  Source, Note, Label, Create, Audio, Video, Deck, Artifact, Guidebook, Generation, Chat,
  Content Transformation, Research, Sharing, Other). Any other value and the command is
  silently dropped from `nlm help`.
- **Custom `help` funcs must not fall through to the generic usage line**
  (`TestCustomCommandHelpDoesNotUseGenericFallback`); `groupedCommandsFromExisting` clones
  entries by name and `mustCommand` **panics** if you rename one without updating the list.
- New tests must stay hermetic — no network, no real credentials, isolated `HOME`.

## Do not run these

- **`gofmt -w` across the tree.** Six committed files already diverge from go1.26 gofmt
  (`cmd/nlm/{betool,chat_stream,label_list,source_match}_test.go`,
  `gen/method/bulk_import_text_source_test.go`, `internal/batchexecute/errors.go`). Format only
  files you edit.
- **`staticcheck` / `golangci-lint`.** ~525 pre-existing findings, mostly `U1000`. Not a gate.
- **Committing binaries or captures.** `/nlm`, `/cmd/nlm/nlm_test`, `*.test`, and
  `docs/captures/` are gitignored deliberately.
- If you ever hit `go: ... directory ../apple does not exist`, a developer `go.work` leaked in;
  it is untracked and absent from clean clones. Use `GOWORK=off`.

## Layout

```
cmd/nlm/            74 .go files. main.go (4290 L, dispatch), commands.go (1715 L — the
                    command table, single source of truth), cli_parser.go, exitcode.go
                    (exit codes 0-7, mirrored in README and `nlm help`), auth.go,
                    betool_*.go (hidden offline wire codec: decode/encode/infer-proto/
                    audit-corpus — the main tool for wire work)
cmd/nlm/testdata/   rsc.io/script txtar CLI tests (*.txt), driven by main_test.go
notebooklm/                  public high-level client, 59 files; client.go is 6172 L
internal/notebooklm/rpc/   low-level batchexecute RPC client + rpc IDs
internal/batchexecute/     batchexecute protocol + error taxonomy
internal/beprotojson/      positional-JSON <-> protobuf codec (1240 L)
internal/rpcinfo/          rpc_id -> proto request/response binding, read from the
                           (rpc_id) descriptor extension (field 51000)
internal/method/           HAR-verified encoders, protected from codegen
internal/nlmmcp/           MCP server (tools.go)
internal/auth/             Chrome/Brave/Edge cookie extraction via CDP
internal/sync/, internal/httprr/, internal/designreview/
gen/notebooklm/v1alpha1/   *.pb.go (orchestration.pb.go alone is 34188 L)
gen/method/ (117)  gen/service/   generated encoders and clients
proto/              *.proto, buf.yaml, buf.gen.yaml, gen.go (go:generate), templates/
docs/               commands.md (gate), EXAMPLES.md, nlm-proto-migration-ledger.md,
                    dev/{index,codegen,test-conventions,http-capture}.md
scripts/            operator shell scripts; not part of build or test
```

Runtime env vars the CLI reads: `NLM_AUTH_TOKEN`, `NLM_COOKIES`, `NLM_AUTHUSER`,
`NLM_BROWSER_PROFILE`, `NLM_CDP_URL`, `NLM_DEBUG`, `NLM_EXPERIMENTAL`, `NLM_AUTO_REFRESH`,
`NLM_HTTP_TRANSPORT`, `NLM_SESSION_ID`, `NLM_BL_PARAM`, `NLM_SIGNALER_AUTH`,
`NLM_SKIP_SOURCES`, `NLM_USE_ORIGINAL_PROFILE`, `NLM_FILE_NAME`.

## Conventions

- Go-project commit messages: `package/path: imperative summary under ~50 chars`, blank line,
  optional body. Match `git log`.
- Errors: lowercase, no trailing punctuation, wrapped as `fmt.Errorf("action: %w", err)`.
- Table-driven tests (`[]struct{name, input, want}`); `github.com/google/go-cmp` for diffs.
- Prefer stdlib; a new dependency needs justification.
- Wire-shape claims need evidence (a HAR capture, a corpus record, or live-use success) cited
  in a comment — an unsourced encoder is indistinguishable from a lucky guess.
- User-facing changes must update `docs/commands.md`, and usually `README.md`.

## Before opening a PR

```bash
go build ./... && go test ./...
go vet ./cmd/... ./internal/... ./proto/...   # must be clean; ./gen/... has a known failure
gofmt -l <only the files you changed>
git status --porcelain   # no binaries, no docs/captures/, no nlm_test
```
