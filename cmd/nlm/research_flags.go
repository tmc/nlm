package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/tmc/nlm/internal/notebooklm/api"
)

type researchOptions struct {
	Mode   string
	MD     bool
	PollMS int
	Import bool
}

func currentResearchOptions() researchOptions {
	return researchOptions{
		Mode:   researchMode,
		MD:     researchMD,
		PollMS: researchPollMs,
		Import: researchImport,
	}
}

func printResearchUsage(cmdName string) {
	fmt.Fprintf(os.Stderr, "Usage: nlm %s [flags] <notebook-id> <query>\n\n", cmdName)
	fmt.Fprintln(os.Stderr, "Flags:")
	fmt.Fprintln(os.Stderr, "  --mode <fast|deep>  Research mode (default: deep)")
	fmt.Fprintln(os.Stderr, "  --md                Emit raw markdown instead of JSON-lines events")
	fmt.Fprintln(os.Stderr, "  --poll-ms <n>       Override deep-research polling interval in milliseconds")
	fmt.Fprintln(os.Stderr, "  --import            Import discovered sources into the notebook after completion")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Examples:")
	fmt.Fprintf(os.Stderr, "  nlm %s <notebook-id> \"What changed in the auth flow?\"\n", cmdName)
	fmt.Fprintf(os.Stderr, "  nlm %s --mode fast <notebook-id> \"Which docs should I read first?\"\n", cmdName)
}

func validateResearchArgs(cmdName string, args []string) error {
	_, positional, err := parseResearchArgs(args)
	if err == nil && len(positional) >= 2 {
		return nil
	}
	fmt.Fprintf(os.Stderr, "usage: nlm %s <notebook-id> <query>\n", cmdName)
	return errBadArgs
}

func parseResearchArgs(args []string) (researchOptions, []string, error) {
	opts := currentResearchOptions()
	flags := flag.NewFlagSet("research", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&opts.Mode, "mode", opts.Mode, "")
	flags.BoolVar(&opts.MD, "md", opts.MD, "")
	flags.IntVar(&opts.PollMS, "poll-ms", opts.PollMS, "")
	flags.BoolVar(&opts.Import, "import", opts.Import, "")

	flagArgs, positional, err := splitCommandFlags(args, map[string]bool{
		"mode":    true,
		"md":      true,
		"poll-ms": true,
		"import":  true,
	}, map[string]bool{
		"md":     true,
		"import": true,
	})
	if err != nil {
		return opts, nil, err
	}
	if err := flags.Parse(flagArgs); err != nil {
		return opts, nil, err
	}
	if len(positional) < 2 {
		return opts, nil, fmt.Errorf("missing notebook id or query")
	}
	if opts.PollMS < 0 {
		return opts, nil, fmt.Errorf("--poll-ms must be >= 0")
	}
	return opts, positional, nil
}

func runResearchCommand(c *api.Client, args []string) error {
	opts, positional, err := parseResearchArgs(args)
	if err != nil {
		return err
	}
	return runResearch(c, positional[0], strings.Join(positional[1:], " "), opts)
}
