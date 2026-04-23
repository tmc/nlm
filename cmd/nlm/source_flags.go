package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/tmc/nlm/internal/notebooklm/api"
	nlmsync "github.com/tmc/nlm/internal/sync"
)

type sourceAddOptions struct {
	Name            string
	MIMEType        string
	ReplaceSourceID string
}

type syncOptions struct {
	Name     string
	Force    bool
	DryRun   bool
	MaxBytes int
	JSON     bool
}

type syncPackOptions struct {
	Name     string
	MaxBytes int
	Chunk    int
}

func printSourceAddUsage(cmdName string) {
	fmt.Fprintf(os.Stderr, "Usage: nlm %s [flags] <notebook-id> <source|-> [source...]\n\n", cmdName)
	fmt.Fprintln(os.Stderr, "Flags:")
	fmt.Fprintln(os.Stderr, "  --name, -n <name>         Custom name for the added source")
	fmt.Fprintln(os.Stderr, "  --mime, --mime-type <t>   Override MIME detection for file/stdin content")
	fmt.Fprintln(os.Stderr, "  --replace <source-id>     Upload a replacement, then delete the old source")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Examples:")
	fmt.Fprintf(os.Stderr, "  nlm %s <notebook-id> https://example.com/article\n", cmdName)
	fmt.Fprintf(os.Stderr, "  nlm %s --name \"API notes\" <notebook-id> ./notes.txt\n", cmdName)
	fmt.Fprintf(os.Stderr, "  nlm %s <notebook-id> -\n", cmdName)
}

func printSourceSyncUsage(cmdName string) {
	fmt.Fprintf(os.Stderr, "Usage: nlm %s [flags] <notebook-id> [paths...]\n\n", cmdName)
	fmt.Fprintln(os.Stderr, "Flags:")
	fmt.Fprintln(os.Stderr, "  --name, -n <name>         Override the generated source title")
	fmt.Fprintln(os.Stderr, "  --force                   Re-upload even when the local hash matches")
	fmt.Fprintln(os.Stderr, "  --dry-run                 Show what would change without uploading")
	fmt.Fprintln(os.Stderr, "  --max-bytes <n>           Chunk threshold in bytes")
	fmt.Fprintln(os.Stderr, "  --json                    Emit NDJSON progress records")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Examples:")
	fmt.Fprintf(os.Stderr, "  nlm %s <notebook-id>\n", cmdName)
	fmt.Fprintf(os.Stderr, "  nlm %s --force <notebook-id> ./docs ./notes\n", cmdName)
	fmt.Fprintf(os.Stderr, "  nlm %s --dry-run <notebook-id> -\n", cmdName)
}

func printSourcePackUsage(cmdName string) {
	fmt.Fprintf(os.Stderr, "Usage: nlm %s [flags] [paths...]\n\n", cmdName)
	fmt.Fprintln(os.Stderr, "Flags:")
	fmt.Fprintln(os.Stderr, "  --name, -n <name>         Override the generated source title")
	fmt.Fprintln(os.Stderr, "  --max-bytes <n>           Chunk threshold in bytes")
	fmt.Fprintln(os.Stderr, "  --chunk <n>               Emit the nth chunk when packing multiple chunks")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Examples:")
	fmt.Fprintf(os.Stderr, "  nlm %s\n", cmdName)
	fmt.Fprintf(os.Stderr, "  nlm %s --chunk 2 ./docs\n", cmdName)
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

	flagArgs, positional, err := splitCommandFlags(args, map[string]bool{
		"name": true, "n": true, "mime": true, "mime-type": true, "replace": true,
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

	flagArgs, positional, err := splitCommandFlags(args, map[string]bool{
		"name": true, "n": true, "force": true, "dry-run": true, "max-bytes": true, "json": true,
	}, map[string]bool{
		"force": true, "dry-run": true, "json": true,
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

	flagArgs, positional, err := splitCommandFlags(args, map[string]bool{
		"name": true, "n": true, "max-bytes": true, "chunk": true,
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
		MaxBytes: opts.MaxBytes,
		Name:     opts.Name,
		Force:    opts.Force,
		DryRun:   opts.DryRun,
		JSON:     opts.JSON,
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
