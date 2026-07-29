package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/tmc/nlm/notebooklm"
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
				withUsage(repeatedOperand("query"), "<query...>"),
			},
			Constraints: []constraint{
				constraintFunc(validateResearchCommand),
			},
		}},
		decodeResearch,
		func(path string) {
			printCommandUsageForPath(path)
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
	return func(_ context.Context, client *notebooklm.Client) error {
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
