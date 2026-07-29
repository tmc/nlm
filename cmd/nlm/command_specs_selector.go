package main

import (
	"context"
	"fmt"

	"github.com/tmc/nlm/notebooklm"
)

type sourceGuideArgs struct {
	NotebookID string
	SourceIDs  []string
	Selectors  selectorOptions
	Force      bool
	JSON       bool
}

func selectorFlagSpecs() []flagSpec {
	return []flagSpec{
		{Name: "source-ids", Value: "ids", Description: "source IDs"},
		{Name: "source-match", Value: "regexp", Description: "source title match"},
		{Name: "source-exclude", Value: "regexp", Description: "source exclusion"},
		{Name: "label-ids", Value: "ids", Description: "label IDs"},
		{Name: "label-match", Value: "regexp", Description: "label name match"},
		{Name: "label-exclude", Value: "regexp", Description: "label exclusion"},
	}
}

func decodeSelectorOptions(parsed parsedCommand) selectorOptions {
	opts := selectorOptionsFromGlobals(parsed.globals)
	opts.SourceIDs = parsedStringFlag(parsed, "source-ids", opts.SourceIDs)
	opts.SourceMatch = parsedStringFlag(parsed, "source-match", opts.SourceMatch)
	opts.SourceExclude = parsedStringFlag(parsed, "source-exclude", opts.SourceExclude)
	opts.LabelIDs = parsedStringFlag(parsed, "label-ids", opts.LabelIDs)
	opts.LabelMatch = parsedStringFlag(parsed, "label-match", opts.LabelMatch)
	opts.LabelExclude = parsedStringFlag(parsed, "label-exclude", opts.LabelExclude)
	return opts
}

func configureSelectorCommandSpecs(specs map[commandID]*commandSpec) {
	spec := specs["source-guide"]
	spec.Flags = selectorFlagSpecs()
	configureTypedCommandSpecWithUsage(spec,
		[]commandForm{{
			Parts: []operandSpec{withUsage(remainingOperand("positionals"), "<notebook-id> [source-id...]")},
			Constraints: []constraint{
				constraintFunc(validateSourceGuideCommand),
			},
		}},
		decodeSourceGuide,
		func(path string) {
			printCommandUsageForPath(path)
		},
	)
}

func validateSourceGuideCommand(parsed parsedCommand) error {
	_, err := decodeSourceGuideArgs(parsed)
	return err
}

func decodeSourceGuide(parsed parsedCommand) (commandCall, error) {
	args, err := decodeSourceGuideArgs(parsed)
	if err != nil {
		return nil, err
	}
	return func(_ context.Context, client *notebooklm.Client) error {
		sourceIDs := args.SourceIDs
		if len(sourceIDs) == 0 {
			var err error
			sourceIDs, err = resolveSourceSelectorsWithOptions(client, args.NotebookID, args.Selectors)
			if err != nil {
				return err
			}
		}
		return generateSourceGuidesWithOptions(client, sourceIDs, args.Force, args.JSON)
	}, nil
}

func decodeSourceGuideArgs(parsed parsedCommand) (sourceGuideArgs, error) {
	positionals := parsed.Args["positionals"]
	if len(positionals) == 0 {
		return sourceGuideArgs{}, fmt.Errorf("missing notebook id")
	}
	selectors := decodeSelectorOptions(parsed)
	sourceIDs := append([]string(nil), positionals[1:]...)
	if len(sourceIDs) == 0 && selectors.empty() {
		return sourceGuideArgs{}, fmt.Errorf("missing source ids or selectors")
	}
	force, err := parsedBoolFlag(parsed, "force", parsed.globals.force)
	if err != nil {
		return sourceGuideArgs{}, err
	}
	jsonOutput, err := parsedBoolFlag(parsed, "json", parsed.globals.jsonOutput)
	if err != nil {
		return sourceGuideArgs{}, err
	}
	return sourceGuideArgs{
		NotebookID: positionals[0],
		SourceIDs:  sourceIDs,
		Selectors:  selectors,
		Force:      force,
		JSON:       jsonOutput,
	}, nil
}
