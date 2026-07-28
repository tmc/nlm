# Releasing nlm

Releases are built locally with GoReleaser. The release creates static binaries
for macOS, Linux, and Windows, publishes archives and checksums to a GitHub
Release, and updates the `nlm` formula in `tmc/homebrew-tap`.

## Prerequisites

- Go 1.25 or newer
- GoReleaser v2
- permission to create a release in `tmc/nlm`
- permission to push a branch and open a pull request in `tmc/homebrew-tap`
- a clean checkout of `main`

Set two GitHub tokens:

```bash
export GITHUB_TOKEN=...
export HOMEBREW_TAP_GITHUB_TOKEN=...
```

`GITHUB_TOKEN` publishes the tag's GitHub Release and its assets in `tmc/nlm`.
`HOMEBREW_TAP_GITHUB_TOKEN` is used only for `tmc/homebrew-tap`. A fine-grained
token should be limited to that repository and grant Contents and Pull requests
write permission. For a classic personal access token, use `public_repo` for
these public repositories. Do not store either token in the repository.

## Prepare

Generated protobuf code is committed. Release builds deliberately do not run
`go generate`: generation invokes a pinned `buf` tool through `go run`, may
download a toolchain and modules, and should not mutate source during a release.
The release hook runs `go mod tidy`, which must leave `go.mod` and `go.sum`
unchanged.

Run the full checks and inspect the release configuration:

```bash
go build ./...
go vet ./...
test -z "$(gofmt -l .)"
go test ./...
goreleaser check
goreleaser release --snapshot --clean
```

Inspect the snapshot archives, checksums, formula, and the output of
`dist/nlm_darwin_arm64*/nlm --version`. A snapshot must not publish anything.

## Publish

Choose the next semantic version. The first release is expected to be `v0.1.0`.
Create an annotated tag and run GoReleaser from the tagged, clean checkout:

```bash
git tag -a v0.1.0 -m 'nlm v0.1.0'
goreleaser release --clean
```

GoReleaser stamps the tag into the binary. `nlm --version` therefore reports
the release version instead of `(devel)` or a source revision.

The release publishes:

- `nlm_Darwin_x86_64.tar.gz`
- `nlm_Darwin_arm64.tar.gz`
- `nlm_Linux_x86_64.tar.gz`
- `nlm_Linux_arm64.tar.gz`
- `nlm_Windows_x86_64.zip`
- `nlm_checksums.txt`
- an `nlm.rb` update on a release branch in `tmc/homebrew-tap`

Review and merge the tap change. Users can then install or upgrade with:

```bash
brew install tmc/tap/nlm
brew upgrade tmc/tap/nlm
```
