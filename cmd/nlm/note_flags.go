package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/tmc/nlm/internal/notebooklm/api"
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

func validateNoteReadArgs(cmdName string, args []string) error {
	_, _, err := parseNoteReadArgs(args)
	if err == nil {
		return nil
	}
	fmt.Fprintf(os.Stderr, "usage: nlm %s [--format text|markdown|html] [--out file] [--open] <notebook-id> <note-id>\n", cmdName)
	return errBadArgs
}

func parseNoteReadArgs(args []string) (noteReadOptions, []string, error) {
	var opts noteReadOptions
	flags := flag.NewFlagSet("note-read", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&opts.Format, "format", "", "")
	flags.StringVar(&opts.OutFile, "out", "", "")
	flags.BoolVar(&opts.Open, "open", false, "")

	flagArgs, positional, err := splitCommandFlags(args, map[string]bool{
		"format": true,
		"out":    true,
		"open":   true,
	}, map[string]bool{
		"open": true,
	})
	if err != nil {
		return opts, nil, err
	}
	if err := flags.Parse(flagArgs); err != nil {
		return opts, nil, err
	}
	if len(positional) != 2 {
		return opts, nil, fmt.Errorf("want notebook id and note id")
	}
	if err := validateNoteFormat(&opts); err != nil {
		return opts, nil, err
	}
	return opts, positional, nil
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

func runNoteRead(c *api.Client, args []string) error {
	opts, positional, err := parseNoteReadArgs(args)
	if err != nil {
		return err
	}
	return readNoteWithOptions(c, positional[0], positional[1], opts)
}
