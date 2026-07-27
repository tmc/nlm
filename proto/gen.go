// Package protobuild holds the NotebookLM service definitions. The generated Go code
// lives in ../gen and is produced by the go:generate directive below.
//
// Regenerate with:
//
//	go generate ./proto
//
// buf and every plugin in buf.gen.yaml run through `go run` at a pinned
// version, so codegen needs no global installs and no Buf Schema Registry
// credentials, and none of it appears in this module's dependency graph.
// The first run builds buf from source and takes a few minutes; later runs
// hit the build cache.
package protobuild

//go:generate go run github.com/bufbuild/buf/cmd/buf@v1.55.1 generate
