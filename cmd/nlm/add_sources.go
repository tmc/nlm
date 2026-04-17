package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/tmc/nlm/internal/notebooklm/api"
)

// addSourceInputs expands the positional source arguments of `nlm add`. A
// single `-` is replaced with newline-delimited entries read from stdin
// (same rules as readLinesFromReader: blank and `#`-prefixed lines dropped,
// first whitespace field wins). Any other value, or a mix, is returned as-is.
//
// The stdin form is only consumed when args is exactly []string{"-"} so that
// literal arguments are never silently replaced by piped input, and so that
// readers like `cat urls.txt | nlm add <nb> -` compose cleanly.
func addSourceInputs(args []string) ([]string, error) {
	if len(args) == 1 && args[0] == "-" {
		if isTerminal(os.Stdin) {
			return nil, fmt.Errorf("refusing to read sources from an interactive stdin; pipe input or pass arguments instead")
		}
		lines, err := readLinesFromReader(os.Stdin)
		if err != nil {
			return nil, err
		}
		if len(lines) == 0 {
			return nil, fmt.Errorf("no sources read from stdin")
		}
		return lines, nil
	}
	return args, nil
}

// validateSourceInputs performs fail-fast checks on every input before any
// RPC call fires. Partial success on batches would be lying — the all-or-
// nothing shape preserves shell-pipe semantics (one non-zero exit = retry
// the whole batch). Rules:
//   - "-" inside a multi-item batch is rejected (stdin can only appear as
//     the sole argument; once we've already expanded it we know we're not
//     in that path).
//   - Inputs looking like URLs (http/https) are accepted as-is; DNS/HTTP
//     failures surface from the RPC.
//   - Inputs with a path separator or that os.Stat can open are treated as
//     files and must exist + be readable.
//   - Anything else is accepted as literal text. Matches the single-arg
//     rule today: `nlm add <nb> hello` = text body "hello"; `nlm add <nb>
//     ./hello` = file (documented in command usage).
func validateSourceInputs(inputs []string) error {
	if len(inputs) == 0 {
		return fmt.Errorf("no sources provided")
	}
	for _, in := range inputs {
		if in == "" {
			return fmt.Errorf("empty source argument")
		}
		if in == "-" {
			return fmt.Errorf("stdin ('-') may only appear as the sole source argument")
		}
		if strings.HasPrefix(in, "http://") || strings.HasPrefix(in, "https://") {
			continue
		}
		// Treat anything with a path separator as a file and require it to
		// exist. Bare tokens fall through to the text-content path.
		if strings.ContainsAny(in, "/\\") || strings.HasSuffix(in, ".txt") || strings.HasSuffix(in, ".md") || strings.HasSuffix(in, ".pdf") {
			if _, err := os.Stat(in); err != nil {
				return fmt.Errorf("source %q: %w", in, err)
			}
		}
	}
	return nil
}

// addSources processes a batch of inputs by calling the existing single-
// source dispatch per entry and emitting one ID per line to stdout. Errors
// are fail-fast: the first failure aborts remaining inputs.
//
// NOTE: the underlying wire path is per-type RPCs (text/base64/upload-url),
// not the izAoDd AddSources bulk envelope — see api.Client.AddSources for
// why. When HAR evidence lands for the bulk wire, this function becomes the
// single switch point.
func addSources(c *api.Client, notebookID string, inputs []string) error {
	if err := validateSourceInputs(inputs); err != nil {
		return err
	}
	for _, in := range inputs {
		id, err := addSource(c, notebookID, in)
		if err != nil {
			return err
		}
		if replaceSourceID != "" && len(inputs) == 1 {
			fmt.Fprintf(os.Stderr, "Replacing source %s...\n", replaceSourceID)
			if delErr := c.DeleteSources(notebookID, []string{replaceSourceID}); delErr != nil {
				fmt.Fprintf(os.Stderr, "warning: uploaded new source but failed to delete old: %v\n", delErr)
			}
		}
		fmt.Println(id)
	}
	return nil
}
