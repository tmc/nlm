package main

import (
	"fmt"
	"os"
	"strings"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/notebooklm/api"
)

// addSourceInputs is the identity function today: positional source args
// pass through unchanged to addSources, which dispatches per entry. The
// single-arg `-` case (stream all of stdin as one source) is handled in
// addSource itself via AddSourceFromReader, preserving the pre-bulk
// semantics where piped input became one blob rather than many lines.
//
// Users who want to add a list of sources from stdin should compose with
// xargs: `cat urls.txt | xargs nlm add <notebook-id>`.
func addSourceInputs(args []string) ([]string, error) {
	return args, nil
}

// validateSourceInputs performs fail-fast checks on every input before any
// RPC call fires. Partial success on batches would be lying — the all-or-
// nothing shape preserves shell-pipe semantics (one non-zero exit = retry
// the whole batch). Rules:
//   - "-" is only valid as the sole argument; it means "stream stdin as
//     one source." Mixing it with other args is rejected.
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
	if len(inputs) == 1 && inputs[0] == "-" {
		if isTerminal(os.Stdin) {
			return fmt.Errorf("refusing to read source from an interactive stdin; pipe input or pass arguments instead")
		}
		return nil
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
func addSources(c *api.Client, notebookID string, inputs []string, opts sourceAddOptions) error {
	if err := validateSourceInputs(inputs); err != nil {
		return err
	}
	knownSourceIDs, _ := sourceIDSet(c, notebookID) // Best-effort cleanup guard.
	for _, in := range inputs {
		id, err := addSource(c, notebookID, in, opts)
		if err != nil {
			if cleanupErr := cleanupFailedAdd(c, notebookID, knownSourceIDs); cleanupErr != nil {
				fmt.Fprintf(os.Stderr, "warning: %v\n", cleanupErr)
			}
			return err
		}
		if knownSourceIDs != nil {
			knownSourceIDs[id] = struct{}{}
		}
		if opts.ReplaceSourceID != "" && len(inputs) == 1 {
			fmt.Fprintf(os.Stderr, "Replacing source %s...\n", opts.ReplaceSourceID)
			if delErr := c.DeleteSources(notebookID, []string{opts.ReplaceSourceID}); delErr != nil {
				fmt.Fprintf(os.Stderr, "warning: uploaded new source but failed to delete old: %v\n", delErr)
			}
		}
		fmt.Println(id)
	}
	return nil
}

func sourceIDSet(c *api.Client, notebookID string) (map[string]struct{}, error) {
	project, err := c.GetProject(notebookID)
	if err != nil {
		return nil, err
	}
	ids := make(map[string]struct{}, len(project.GetSources()))
	for _, src := range project.GetSources() {
		id := src.GetSourceId().GetSourceId()
		if id == "" {
			continue
		}
		ids[id] = struct{}{}
	}
	return ids, nil
}

func cleanupFailedAdd(c *api.Client, notebookID string, knownSourceIDs map[string]struct{}) error {
	if knownSourceIDs == nil {
		return nil
	}
	project, err := c.GetProject(notebookID)
	if err != nil {
		return fmt.Errorf("could not inspect sources after failed add: %w", err)
	}
	stale := staleFailedAddSourceIDs(knownSourceIDs, project)
	if len(stale) == 0 {
		return nil
	}
	if err := c.DeleteSources(notebookID, stale); err != nil {
		return fmt.Errorf("failed add left stale error source IDs %s; remove them with `nlm rm-source %s %s`: %w",
			strings.Join(stale, ","),
			notebookID,
			strings.Join(stale, ","),
			err,
		)
	}
	if len(stale) == 1 {
		fmt.Fprintf(os.Stderr, "Removed stale failed source record: %s\n", stale[0])
		return nil
	}
	fmt.Fprintf(os.Stderr, "Removed %d stale failed source records\n", len(stale))
	return nil
}

func staleFailedAddSourceIDs(knownSourceIDs map[string]struct{}, project *pb.Project) []string {
	var stale []string
	for _, src := range project.GetSources() {
		id := src.GetSourceId().GetSourceId()
		if id == "" {
			continue
		}
		if _, ok := knownSourceIDs[id]; ok {
			continue
		}
		if !sourceHasErrorStatus(src) {
			continue
		}
		stale = append(stale, id)
	}
	return stale
}

func sourceHasErrorStatus(src *pb.Source) bool {
	return src.GetSettings().GetStatus() == pb.SourceSettings_SOURCE_STATUS_ERROR ||
		src.GetMetadata().GetStatus() == pb.SourceSettings_SOURCE_STATUS_ERROR
}
