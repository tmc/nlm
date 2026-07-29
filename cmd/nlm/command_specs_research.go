package main

import (
	"context"
	"fmt"
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

type researchArgs struct {
	NotebookID string
	Query      string
	Options    researchOptions
}

func printResearchUsage(cmdName string) {
	fmt.Fprintf(os.Stderr, "Usage: nlm %s [flags] <notebook-id> <query>\n\n", cmdName)
	fmt.Fprintln(os.Stderr, "Flags:")
	fmt.Fprintln(os.Stderr, "  --mode <fast|deep>  Research mode (default: deep)")
	fmt.Fprintln(os.Stderr, "  --md                Emit Markdown with source footnotes instead of JSON-lines")
	fmt.Fprintln(os.Stderr, "  --poll-ms <n>       Override deep-research polling interval in milliseconds")
	fmt.Fprintln(os.Stderr, "  --import            Import discovered sources into the notebook after completion")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Examples:")
	fmt.Fprintf(os.Stderr, "  nlm %s <notebook-id> \"What changed in the auth flow?\"\n", cmdName)
	fmt.Fprintf(os.Stderr, "  nlm %s --mode fast <notebook-id> \"Which docs should I read first?\"\n", cmdName)
}

func configureResearchCommandSpecs(specs map[commandID]*commandSpec) {
	spec := specs["research"]
	spec.Flags = []flagSpec{
		{Name: "mode", Value: "fast|deep", Description: "research mode"},
		{Name: "md", Description: "emit markdown"},
		{Name: "poll-ms", Value: "n", Description: "poll interval"},
		{Name: "import", Description: "import sources"},
	}
	configureTypedCommandSpecWithUsage(spec,
		[]commandForm{{
			Parts: []operandSpec{
				requiredOperand("notebook"),
				repeatedOperand("query"),
			},
			Constraints: []constraint{
				constraintFunc(validateResearchCommand),
			},
		}},
		decodeResearch,
		func(path string) {
			fmt.Fprintf(os.Stderr, "usage: nlm %s <notebook-id> <query>\n", path)
		},
	)
}

func validateResearchCommand(parsed parsedCommand) error {
	_, err := decodeResearchArgs(parsed)
	return err
}

func decodeResearch(parsed parsedCommand) (commandCall, error) {
	args, err := decodeResearchArgs(parsed)
	if err != nil {
		return nil, err
	}
	return func(_ context.Context, client *api.Client) error {
		return runResearch(client, args.NotebookID, args.Query, args.Options)
	}, nil
}

func decodeResearchArgs(parsed parsedCommand) (researchArgs, error) {
	notebookID, err := parsedArgument(parsed, "notebook")
	if err != nil {
		return researchArgs{}, err
	}
	query, err := parsedArguments(parsed, "query")
	if err != nil {
		return researchArgs{}, err
	}
	md, err := parsedBoolFlag(parsed, "md", parsed.globals.researchMD)
	if err != nil {
		return researchArgs{}, err
	}
	pollMS, err := parsedIntFlag(parsed, "poll-ms", parsed.globals.researchPollMs)
	if err != nil {
		return researchArgs{}, err
	}
	importSources, err := parsedBoolFlag(parsed, "import", parsed.globals.researchImport)
	if err != nil {
		return researchArgs{}, err
	}
	if pollMS < 0 {
		return researchArgs{}, fmt.Errorf("--poll-ms must be >= 0")
	}
	return researchArgs{
		NotebookID: notebookID,
		Query:      strings.Join(query, " "),
		Options: researchOptions{
			Mode:   parsedStringFlag(parsed, "mode", parsed.globals.researchMode),
			MD:     md,
			PollMS: pollMS,
			Import: importSources,
		},
	}, nil
}
