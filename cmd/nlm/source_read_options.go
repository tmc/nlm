package main

import (
	"fmt"
	"io"
	"os"
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
