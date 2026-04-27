package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/tmc/nlm/internal/notebooklm/api"
)

type sourceSelectionOptions struct {
	Selectors selectorOptions
}

func printSourceSelectionUsage(cmdName string) {
	fmt.Fprintf(os.Stderr, "Usage: nlm %s [flags] <notebook-id> [source-id...]\n\n", cmdName)
	fmt.Fprintln(os.Stderr, "Flags:")
	fmt.Fprintln(os.Stderr, "  --source-ids <ids>       Focus on these source IDs ('a,b,c' or '-' for stdin)")
	fmt.Fprintln(os.Stderr, "  --source-match <regex>   Focus on sources whose title or UUID matches the regex")
	fmt.Fprintln(os.Stderr, "  --source-exclude <regex> Exclude sources whose title or UUID matches the regex")
	fmt.Fprintln(os.Stderr, "  --label-ids <ids>        Include sources tagged with any of these label IDs")
	fmt.Fprintln(os.Stderr, "  --label-match <regex>    Include sources tagged with any label whose name matches the regex")
	fmt.Fprintln(os.Stderr, "  --label-exclude <regex>  Exclude sources tagged with any label whose name matches the regex")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Examples:")
	fmt.Fprintf(os.Stderr, "  nlm %s <notebook-id> <source-id>\n", cmdName)
	fmt.Fprintf(os.Stderr, "  nlm %s --source-match '^spec/' <notebook-id>\n", cmdName)
	fmt.Fprintf(os.Stderr, "  nlm %s --source-match '^spec/' --source-exclude 'draft' <notebook-id>\n", cmdName)
	fmt.Fprintf(os.Stderr, "  nlm %s --label-match '^Testing$' <notebook-id>\n", cmdName)
}

func validateSourceSelectionArgs(cmdName string, args []string) error {
	_, _, err := parseSourceSelectionArgs(args)
	if err == nil {
		return nil
	}
	fmt.Fprintf(os.Stderr, "usage: nlm %s <notebook-id> <source-id> [source-id...] (or pass --source-ids / --source-match)\n", cmdName)
	return errBadArgs
}

func parseSourceSelectionArgs(args []string) (sourceSelectionOptions, []string, error) {
	opts := sourceSelectionOptions{
		Selectors: currentSelectorOptions(),
	}
	flags := flag.NewFlagSet("source-selection", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&opts.Selectors.SourceIDs, "source-ids", opts.Selectors.SourceIDs, "")
	flags.StringVar(&opts.Selectors.SourceMatch, "source-match", opts.Selectors.SourceMatch, "")
	flags.StringVar(&opts.Selectors.SourceExclude, "source-exclude", opts.Selectors.SourceExclude, "")
	flags.StringVar(&opts.Selectors.LabelIDs, "label-ids", opts.Selectors.LabelIDs, "")
	flags.StringVar(&opts.Selectors.LabelMatch, "label-match", opts.Selectors.LabelMatch, "")
	flags.StringVar(&opts.Selectors.LabelExclude, "label-exclude", opts.Selectors.LabelExclude, "")

	flagArgs, positional, err := splitCommandFlags(args, map[string]bool{
		"source-ids":     true,
		"source-match":   true,
		"source-exclude": true,
		"label-ids":      true,
		"label-match":    true,
		"label-exclude":  true,
	}, nil)
	if err != nil {
		return opts, nil, err
	}
	if err := flags.Parse(flagArgs); err != nil {
		return opts, nil, err
	}
	if len(positional) == 0 {
		return opts, nil, fmt.Errorf("missing notebook id")
	}
	if len(positional) < 2 && opts.Selectors.empty() {
		return opts, nil, fmt.Errorf("missing source ids or selectors")
	}
	return opts, positional, nil
}

func runSourceSelectionAction(c *api.Client, args []string, action string) error {
	opts, positional, err := parseSourceSelectionArgs(args)
	if err != nil {
		return err
	}
	notebookID := positional[0]
	sourceIDs := positional[1:]
	if len(sourceIDs) == 0 {
		sourceIDs, err = resolveSourceSelectorsWithOptions(c, notebookID, opts.Selectors)
		if err != nil {
			return err
		}
	}
	return actOnSources(c, notebookID, action, sourceIDs)
}

func runSourceGuide(c *api.Client, args []string) error {
	opts, positional, err := parseSourceSelectionArgs(args)
	if err != nil {
		return err
	}
	notebookID := positional[0]
	sourceIDs := positional[1:]
	if len(sourceIDs) == 0 {
		sourceIDs, err = resolveSourceSelectorsWithOptions(c, notebookID, opts.Selectors)
		if err != nil {
			return err
		}
	}
	return generateSourceGuides(c, sourceIDs)
}
