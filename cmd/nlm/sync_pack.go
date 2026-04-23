package main

import (
	"fmt"
	"os"

	nlmsync "github.com/tmc/nlm/internal/sync"
)

// runSyncPack writes the txtar bytes that `nlm sync` would upload, without
// touching the network. It reuses the same discover/quote/bundle pipeline
// as sync so savvy users can pipe the output through `txtar --list` or
// `txtar -x` to inspect exactly what will land in the notebook.
//
// Path semantics match sync: zero args means the current directory.
// When the bundle fits in a single chunk, the bytes go to stdout.
// Multi-chunk bundles list their names on stderr and require --chunk N
// to select which one to emit on stdout.
func runSyncPack(args []string, opts syncPackOptions) error {
	paths := args
	if len(paths) == 0 {
		paths = []string{"."}
	}
	if len(paths) == 1 && paths[0] == "-" {
		paths = nil
	}

	packOpts := nlmsync.Options{
		MaxBytes: opts.MaxBytes,
		Name:     opts.Name,
	}
	chunks, names, err := nlmsync.Pack(paths, packOpts)
	if err != nil {
		return err
	}

	switch {
	case opts.Chunk > 0:
		if opts.Chunk > len(chunks) {
			return fmt.Errorf("--chunk %d out of range (have %d chunks)", opts.Chunk, len(chunks))
		}
		_, err := os.Stdout.Write(chunks[opts.Chunk-1])
		return err
	case len(chunks) == 1:
		_, err := os.Stdout.Write(chunks[0])
		return err
	default:
		fmt.Fprintf(os.Stderr, "bundle has %d chunks; pass --chunk N to emit one:\n", len(chunks))
		for i, n := range names {
			fmt.Fprintf(os.Stderr, "  %d: %s (%d bytes)\n", i+1, n, len(chunks[i]))
		}
		return fmt.Errorf("multiple chunks; --chunk required")
	}
}
