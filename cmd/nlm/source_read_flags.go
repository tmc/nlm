package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/tmc/nlm/internal/notebooklm/api"
)

func printSourceReadUsage(cmdName string) {
	fmt.Fprintf(os.Stderr, "Usage: nlm %s [flags] <source-id> [notebook-id]\n\n", cmdName)
	fmt.Fprintln(os.Stderr, "Flags:")
	fmt.Fprintln(os.Stderr, "  --format <fmt>  Output format: text (default), markdown, html, json, or raw")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "The json format is nlm's stable decoded source model. The raw format is")
	fmt.Fprintln(os.Stderr, "the unstable LoadSource protobuf encoded with protojson.")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Deprecated aliases: --markdown, --html, and --json.")
}

func validateSourceReadArgsWithOptions(cmdName string, args []string, globals globalOptions) error {
	_, _, err := parseSourceReadArgs(args, globals)
	if err == nil {
		return nil
	}
	fmt.Fprintf(os.Stderr, "usage: nlm %s [--format text|markdown|html|json|raw] <source-id> [notebook-id]\n", cmdName)
	return errBadArgs
}

func parseSourceReadArgs(args []string, globals globalOptions) (globalOptions, []string, error) {
	opts := globals
	flags := flag.NewFlagSet("source-read", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&opts.sourceReadFormat, "format", opts.sourceReadFormat, "")

	flagArgs, positional, err := splitCommandFlags(args, map[string]bool{
		"format": true,
	}, nil)
	if err != nil {
		return opts, nil, err
	}
	if err := flags.Parse(flagArgs); err != nil {
		return opts, nil, err
	}
	if len(positional) < 1 || len(positional) > 2 {
		return opts, nil, fmt.Errorf("want source id and optional notebook id")
	}
	if err := normalizeSourceReadFormat(&opts); err != nil {
		return opts, nil, err
	}
	return opts, positional, nil
}

func normalizeSourceReadFormat(opts *globalOptions) error {
	selected := 0
	for _, value := range []bool{
		opts.sourceReadFormat != "",
		opts.jsonOutput,
		opts.sourceReadMarkdown,
		opts.sourceReadHTML,
	} {
		if value {
			selected++
		}
	}
	if selected > 1 {
		return fmt.Errorf("source read: use only one of --format, --json, --markdown, and --html")
	}

	format := opts.sourceReadFormat
	switch {
	case opts.jsonOutput:
		format = "json"
	case opts.sourceReadMarkdown:
		format = "markdown"
	case opts.sourceReadHTML:
		format = "html"
	}
	switch format {
	case "", "text":
		format = "text"
	case "markdown", "md":
		format = "markdown"
	case "html", "json", "raw":
	case "raw-json":
		format = "raw"
	default:
		return fmt.Errorf("unknown --format %q (want text, markdown, html, json, or raw)", format)
	}
	opts.sourceReadFormat = format
	opts.jsonOutput = false
	opts.sourceReadMarkdown = false
	opts.sourceReadHTML = false
	return nil
}

func runSourceRead(c *api.Client, args []string, globals globalOptions) error {
	opts, positional, err := parseSourceReadArgs(args, globals)
	if err != nil {
		return err
	}
	warnDeprecatedSourceReadFormat(os.Stderr, globals)
	return readSource(c, positional, opts)
}

func warnDeprecatedSourceReadFormat(w io.Writer, opts globalOptions) {
	switch {
	case opts.jsonOutput:
		fmt.Fprintln(w, "nlm: source read: --json is deprecated; use --format=json")
	case opts.sourceReadMarkdown:
		fmt.Fprintln(w, "nlm: source read: --markdown is deprecated; use --format=markdown")
	case opts.sourceReadHTML:
		fmt.Fprintln(w, "nlm: source read: --html is deprecated; use --format=html")
	}
}
