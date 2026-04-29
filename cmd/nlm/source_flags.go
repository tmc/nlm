package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/tmc/nlm/internal/notebooklm/api"
	nlmsync "github.com/tmc/nlm/internal/sync"
)

// stringSliceFlag collects repeated --flag values into a slice.
type stringSliceFlag []string

func (s *stringSliceFlag) String() string { return strings.Join(*s, ",") }

func (s *stringSliceFlag) Set(v string) error {
	*s = append(*s, v)
	return nil
}

type sourceAddOptions struct {
	Name            string
	MIMEType        string
	ReplaceSourceID string
	// PreProcess is a shell command run for each non-URL source. The source
	// content is piped to its stdin; stdout replaces what gets uploaded. A
	// non-zero exit status aborts the batch.
	PreProcess string
	// Chunk, if > 0, splits each non-URL source into parts of at most this
	// many bytes each. The first part keeps the source name; subsequent
	// parts are named "<name> (pt2)", "<name> (pt3)", ... Matches the
	// naming scheme `nlm sync` uses for bundle chunks.
	Chunk int
}

type syncOptions struct {
	Name             string
	Force            bool
	DryRun           bool
	MaxBytes         int
	JSON             bool
	Exclude          []string
	IncludeUntracked bool
}

type syncPackOptions struct {
	Name     string
	MaxBytes int
	Chunk    int
	Exclude  []string
}

func printSourceAddUsage(cmdName string) {
	fmt.Fprintf(os.Stderr, "Usage: nlm %s [flags] <notebook-id> <source|-> [source...]\n\n", cmdName)
	fmt.Fprintln(os.Stderr, "Sources may be files, URLs, or text literals. A sole '-' streams all of")
	fmt.Fprintln(os.Stderr, "stdin in as a single source (pair with --name and --mime-type). To add a")
	fmt.Fprintln(os.Stderr, "list of sources from stdin, compose with xargs.")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Flags:")
	fmt.Fprintln(os.Stderr, "  --name, -n <name>         Custom name for the added source")
	fmt.Fprintln(os.Stderr, "  --mime, --mime-type <t>   Override MIME detection for file/stdin content")
	fmt.Fprintln(os.Stderr, "  --replace <source-id>     Upload a replacement, then delete the old source")
	fmt.Fprintln(os.Stderr, "  --pre-process <cmd>       Pipe each non-URL source through 'sh -c cmd' before")
	fmt.Fprintln(os.Stderr, "                            upload; stdout replaces the content. Non-zero exit")
	fmt.Fprintln(os.Stderr, "                            aborts the batch. URL sources are passed through.")
	fmt.Fprintln(os.Stderr, "  --chunk <bytes>           Split each non-URL source into parts of at most <bytes>")
	fmt.Fprintln(os.Stderr, "                            each. Parts upload as \"name\", \"name (pt2)\", ... Use for")
	fmt.Fprintln(os.Stderr, "                            content that exceeds the per-request size limit without")
	fmt.Fprintln(os.Stderr, "                            switching to `nlm sync` txtar bundling.")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Examples:")
	fmt.Fprintf(os.Stderr, "  nlm %s <notebook-id> https://example.com/article\n", cmdName)
	fmt.Fprintf(os.Stderr, "  nlm %s --name \"API notes\" <notebook-id> ./notes.txt\n", cmdName)
	fmt.Fprintf(os.Stderr, "  cat notes.md | nlm %s --name \"April notes\" <notebook-id> -\n", cmdName)
	fmt.Fprintf(os.Stderr, "  cat urls.txt | xargs nlm %s <notebook-id>\n", cmdName)
	fmt.Fprintf(os.Stderr, "  nlm %s --pre-process 'pandoc -f docx -t markdown' <notebook-id> brief.docx\n", cmdName)
	fmt.Fprintf(os.Stderr, "  nlm %s --chunk 5242880 <notebook-id> huge.log\n", cmdName)
}

func printSourceSyncUsage(cmdName string) {
	fmt.Fprintf(os.Stderr, "Usage: nlm %s [flags] <notebook-id> [paths...]\n\n", cmdName)
	if cmdName == "source sync" {
		fmt.Fprintln(os.Stderr, "(also available as the top-level shortcut: nlm sync)")
		fmt.Fprintln(os.Stderr)
	}
	fmt.Fprintln(os.Stderr, "Bundles local files into a txtar archive and uploads them as a single named")
	fmt.Fprintln(os.Stderr, "source. Re-running sync updates that source in place: unchanged content is")
	fmt.Fprintln(os.Stderr, "skipped via a hash cache, and archives larger than --max-bytes are split into")
	fmt.Fprintln(os.Stderr, "numbered parts (\"name\", \"name (pt2)\", ...).")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Path handling:")
	fmt.Fprintln(os.Stderr, "  (no paths)                Sync the current directory")
	fmt.Fprintln(os.Stderr, "  <dir>                     Include files tracked by git ls-files (falls back")
	fmt.Fprintln(os.Stderr, "                            to a recursive walk; skips .git, node_modules,")
	fmt.Fprintln(os.Stderr, "                            __pycache__, .eggs)")
	fmt.Fprintln(os.Stderr, "  <file>                    Include that file verbatim")
	fmt.Fprintln(os.Stderr, "  -                         Read newline-delimited paths from stdin")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Binary files are detected and skipped. Text files containing lines that look")
	fmt.Fprintln(os.Stderr, "like txtar markers are safely quoted so the archive round-trips.")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Flags:")
	fmt.Fprintln(os.Stderr, "  --name, -n <name>         Source title (defaults to the basename of the")
	fmt.Fprintln(os.Stderr, "                            single path; required with multiple paths or stdin)")
	fmt.Fprintln(os.Stderr, "  --force                   Re-upload even when the content hash is unchanged")
	fmt.Fprintln(os.Stderr, "  --dry-run                 Print the plan (add/update/skip/delete) without")
	fmt.Fprintln(os.Stderr, "                            contacting the server")
	fmt.Fprintln(os.Stderr, "  --max-bytes <n>           Per-chunk size threshold (default 5120000)")
	fmt.Fprintln(os.Stderr, "  --json                    Emit NDJSON progress records instead of text")
	fmt.Fprintln(os.Stderr, "  --exclude <pattern>       Skip files matching a filepath.Match pattern;")
	fmt.Fprintln(os.Stderr, "                            tested against the full path and basename. May")
	fmt.Fprintln(os.Stderr, "                            be repeated. Trailing '/' or '/'-bearing patterns")
	fmt.Fprintln(os.Stderr, "                            match as path prefixes (e.g. 'vendor/')")
	fmt.Fprintln(os.Stderr, "  --include-untracked       Include untracked, non-ignored files when syncing")
	fmt.Fprintln(os.Stderr, "                            git directories")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Hash cache: ~/.cache/nlm/sync/<notebook-id>/")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Examples:")
	fmt.Fprintf(os.Stderr, "  nlm %s <notebook-id>                    # sync the current directory\n", cmdName)
	fmt.Fprintf(os.Stderr, "  nlm %s -n docs <notebook-id> ./docs ./notes\n", cmdName)
	fmt.Fprintf(os.Stderr, "  nlm %s --dry-run <notebook-id>          # preview without uploading\n", cmdName)
	fmt.Fprintf(os.Stderr, "  nlm %s --force <notebook-id> README.md  # force re-upload\n", cmdName)
	fmt.Fprintf(os.Stderr, "  nlm %s --exclude '*.pb.go' --exclude 'vendor/' <notebook-id>\n", cmdName)
	fmt.Fprintf(os.Stderr, "  git ls-files '*.go' | nlm %s -n go-src <notebook-id> -\n", cmdName)
}

func printSourcePackUsage(cmdName string) {
	fmt.Fprintf(os.Stderr, "Usage: nlm %s [flags] [paths...]\n\n", cmdName)
	if cmdName == "source pack" {
		fmt.Fprintln(os.Stderr, "(also available as the top-level shortcut: nlm sync-pack)")
		fmt.Fprintln(os.Stderr)
	}
	fmt.Fprintln(os.Stderr, "Runs the same discover/bundle pipeline as sync but writes the resulting txtar")
	fmt.Fprintln(os.Stderr, "archive to stdout without contacting the server. Useful for previewing what")
	fmt.Fprintln(os.Stderr, "sync would upload, or for piping into tools that consume txtar.")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "With no --chunk flag: emits the sole chunk, or lists chunk sizes to stderr")
	fmt.Fprintln(os.Stderr, "when the bundle would be split. Pass --chunk N to emit the Nth chunk.")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Flags:")
	fmt.Fprintln(os.Stderr, "  --name, -n <name>         Source title (same rules as sync)")
	fmt.Fprintln(os.Stderr, "  --max-bytes <n>           Per-chunk size threshold (default 5120000)")
	fmt.Fprintln(os.Stderr, "  --chunk <n>               Emit the Nth chunk (1-indexed) when multiple")
	fmt.Fprintln(os.Stderr, "  --exclude <pattern>       Skip files matching the pattern (repeatable;")
	fmt.Fprintln(os.Stderr, "                            same rules as sync)")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Examples:")
	fmt.Fprintf(os.Stderr, "  nlm %s                                  # pack the current directory\n", cmdName)
	fmt.Fprintf(os.Stderr, "  nlm %s ./docs > docs.txtar\n", cmdName)
	fmt.Fprintf(os.Stderr, "  nlm %s --chunk 2 ./docs\n", cmdName)
	fmt.Fprintf(os.Stderr, "  nlm %s --exclude '*.pb.go' ./src\n", cmdName)
}

func validateSourceAddArgs(cmdName string, args []string) error {
	_, _, _, err := parseSourceAddArgs(args)
	if err == nil {
		return nil
	}
	fmt.Fprintf(os.Stderr, "usage: nlm %s <notebook-id> <source|-> [source...]\n", cmdName)
	return errBadArgs
}

func validateSourceSyncArgs(cmdName string, args []string) error {
	_, _, err := parseSourceSyncArgs(args)
	if err == nil {
		return nil
	}
	fmt.Fprintf(os.Stderr, "usage: nlm %s <notebook-id> [paths...]\n", cmdName)
	return errBadArgs
}

func validateSourcePackArgs(cmdName string, args []string) error {
	_, _, err := parseSourcePackArgs(args)
	if err == nil {
		return nil
	}
	fmt.Fprintf(os.Stderr, "usage: nlm %s [paths...]\n", cmdName)
	return errBadArgs
}

func parseSourceAddArgs(args []string) (sourceAddOptions, string, []string, error) {
	opts := sourceAddOptions{
		Name:            sourceName,
		MIMEType:        mimeType,
		ReplaceSourceID: replaceSourceID,
	}
	flags := flag.NewFlagSet("source-add", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&opts.Name, "name", opts.Name, "")
	flags.StringVar(&opts.Name, "n", opts.Name, "")
	flags.StringVar(&opts.MIMEType, "mime", opts.MIMEType, "")
	flags.StringVar(&opts.MIMEType, "mime-type", opts.MIMEType, "")
	flags.StringVar(&opts.ReplaceSourceID, "replace", opts.ReplaceSourceID, "")
	flags.StringVar(&opts.PreProcess, "pre-process", opts.PreProcess, "")
	flags.IntVar(&opts.Chunk, "chunk", opts.Chunk, "")

	flagArgs, positional, err := splitCommandFlags(args, map[string]bool{
		"name": true, "n": true, "mime": true, "mime-type": true, "replace": true,
		"pre-process": true, "chunk": true,
	}, nil)
	if err != nil {
		return opts, "", nil, err
	}
	if err := flags.Parse(flagArgs); err != nil {
		return opts, "", nil, err
	}
	if len(positional) < 2 {
		return opts, "", nil, fmt.Errorf("missing notebook id or source")
	}
	if opts.Chunk < 0 {
		return opts, "", nil, fmt.Errorf("--chunk must be >= 0")
	}
	if opts.Chunk > api.MaxTextSourceBytes {
		return opts, "", nil, fmt.Errorf("--chunk %d exceeds per-request limit %d", opts.Chunk, api.MaxTextSourceBytes)
	}
	return opts, positional[0], positional[1:], nil
}

func parseSourceSyncArgs(args []string) (syncOptions, []string, error) {
	opts := syncOptions{
		Name:     sourceName,
		Force:    force,
		DryRun:   dryRun,
		MaxBytes: maxBytes,
		JSON:     jsonOutput,
	}
	flags := flag.NewFlagSet("source-sync", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&opts.Name, "name", opts.Name, "")
	flags.StringVar(&opts.Name, "n", opts.Name, "")
	flags.BoolVar(&opts.Force, "force", opts.Force, "")
	flags.BoolVar(&opts.DryRun, "dry-run", opts.DryRun, "")
	flags.IntVar(&opts.MaxBytes, "max-bytes", opts.MaxBytes, "")
	flags.BoolVar(&opts.JSON, "json", opts.JSON, "")
	flags.BoolVar(&opts.IncludeUntracked, "include-untracked", opts.IncludeUntracked, "")
	excludes := (*stringSliceFlag)(&opts.Exclude)
	flags.Var(excludes, "exclude", "")
	flags.Var(excludes, "x", "")

	flagArgs, positional, err := splitCommandFlags(args, map[string]bool{
		"name": true, "n": true, "force": true, "dry-run": true, "max-bytes": true, "json": true,
		"exclude": true, "x": true, "include-untracked": true,
	}, map[string]bool{
		"force": true, "dry-run": true, "json": true, "include-untracked": true,
	})
	if err != nil {
		return opts, nil, err
	}
	if err := flags.Parse(flagArgs); err != nil {
		return opts, nil, err
	}
	if len(positional) == 0 {
		return opts, nil, fmt.Errorf("missing notebook id")
	}
	if opts.MaxBytes < 0 {
		return opts, nil, fmt.Errorf("--max-bytes must be >= 0")
	}
	return opts, positional, nil
}

func parseSourcePackArgs(args []string) (syncPackOptions, []string, error) {
	opts := syncPackOptions{
		Name:     sourceName,
		MaxBytes: maxBytes,
		Chunk:    packChunk,
	}
	flags := flag.NewFlagSet("source-pack", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&opts.Name, "name", opts.Name, "")
	flags.StringVar(&opts.Name, "n", opts.Name, "")
	flags.IntVar(&opts.MaxBytes, "max-bytes", opts.MaxBytes, "")
	flags.IntVar(&opts.Chunk, "chunk", opts.Chunk, "")
	excludes := (*stringSliceFlag)(&opts.Exclude)
	flags.Var(excludes, "exclude", "")
	flags.Var(excludes, "x", "")

	flagArgs, positional, err := splitCommandFlags(args, map[string]bool{
		"name": true, "n": true, "max-bytes": true, "chunk": true,
		"exclude": true, "x": true,
	}, nil)
	if err != nil {
		return opts, nil, err
	}
	if err := flags.Parse(flagArgs); err != nil {
		return opts, nil, err
	}
	if opts.MaxBytes < 0 {
		return opts, nil, fmt.Errorf("--max-bytes must be >= 0")
	}
	if opts.Chunk < 0 {
		return opts, nil, fmt.Errorf("--chunk must be >= 0")
	}
	return opts, positional, nil
}

func runSourceAdd(c *api.Client, args []string) error {
	opts, notebookID, rawInputs, err := parseSourceAddArgs(args)
	if err != nil {
		return err
	}
	inputs, err := addSourceInputs(rawInputs)
	if err != nil {
		return err
	}
	return addSources(c, notebookID, inputs, opts)
}

func runSourceSync(c *api.Client, args []string) error {
	opts, positional, err := parseSourceSyncArgs(args)
	if err != nil {
		return err
	}
	notebookID := positional[0]
	var paths []string
	if len(positional) > 1 {
		if positional[1] == "-" {
			paths = nil
		} else {
			paths = positional[1:]
		}
	} else {
		paths = []string{"."}
	}
	syncOpts := nlmsync.Options{
		MaxBytes:         opts.MaxBytes,
		Name:             opts.Name,
		Force:            opts.Force,
		DryRun:           opts.DryRun,
		JSON:             opts.JSON,
		Exclude:          opts.Exclude,
		IncludeUntracked: opts.IncludeUntracked,
	}
	adapter := &syncClientAdapter{client: c}
	return nlmsync.Run(context.Background(), adapter, notebookID, paths, syncOpts, os.Stdout)
}

func runSourcePack(args []string) error {
	opts, paths, err := parseSourcePackArgs(args)
	if err != nil {
		return err
	}
	return runSyncPack(paths, opts)
}
