package main

import (
	"fmt"
	"os"
)

type noteReadOptions struct {
	Format  string
	OutFile string
	Open    bool
}

func printNoteReadUsage(cmdName string) {
	fmt.Fprintf(os.Stderr, "Usage: nlm %s [flags] <notebook-id> <note-id>\n\n", cmdName)
	fmt.Fprintln(os.Stderr, "Flags:")
	fmt.Fprintln(os.Stderr, "  --format <fmt>  Output format: text (default), markdown, or html")
	fmt.Fprintln(os.Stderr, "  --out <file>    Write html output to a file instead of stdout (--format=html only)")
	fmt.Fprintln(os.Stderr, "  --open          Open the written html file in a browser (--format=html with --out)")
}

func validateNoteFormat(opts *noteReadOptions) error {
	switch opts.Format {
	case "", "text":
		opts.Format = "text"
	case "markdown", "md":
		opts.Format = "markdown"
	case "html":
		opts.Format = "html"
	default:
		return fmt.Errorf("unknown --format %q (want text, markdown, or html)", opts.Format)
	}
	if opts.Format != "html" {
		if opts.OutFile != "" {
			return fmt.Errorf("--out only applies to --format=html")
		}
		if opts.Open {
			return fmt.Errorf("--open only applies to --format=html")
		}
	}
	return nil
}
