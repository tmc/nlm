package main

import (
	"context"
	"fmt"
	"os"

	"github.com/tmc/nlm/internal/notebooklm/api"
)

type artifactIDArgs struct {
	ArtifactID string
	Yes        bool
}

type artifactReadArgs struct {
	ArtifactID string
	CDPURL     string
}

type artifactListArgs struct {
	NotebookID string
	JSON       bool
}

type artifactRenameArgs struct {
	ArtifactID string
	Title      string
}

type updateArtifactOptions struct {
	Name string
}

type artifactUpdateArgs struct {
	ArtifactID string
	Options    updateArtifactOptions
}

type artifactExportArgs struct {
	ArtifactID string
	Options    artifactExportOptions
}

func configureArtifactCommandSpecs(specs map[commandID]*commandSpec) {
	artifactForm := commandFormOf(requiredOperand("artifact"))
	configureTypedCommandSpec(specs["get-artifact"], artifactForm, decodeArtifactGet)
	configureTypedCommandSpec(specs["read-artifact"], artifactForm, decodeArtifactRead)
	configureTypedCommandSpec(specs["artifacts"],
		commandFormOf(requiredOperand("notebook")),
		decodeArtifactList,
	)
	exportSpec := specs["export-flashcards"]
	exportSpec.Flags = []flagSpec{
		{Name: "format", Aliases: []string{"f"}, Value: "format", Description: "artifact format"},
		{Name: "output", Aliases: []string{"o"}, Value: "file", Description: "output file"},
	}
	configureTypedCommandSpecWithErrorUsage(exportSpec,
		[]commandForm{{
			Parts: []operandSpec{withUsage(remainingOperand("artifacts"), "<artifact-id>")},
			Constraints: []constraint{
				constraintFunc(validateArtifactExportCommand),
			},
		}},
		decodeArtifactExport,
		func(path string, err error) {
			fmt.Fprintf(os.Stderr, "nlm: %s: %v\n\n", path, err)
			printCommandHelpForPath(path)
		},
	)
	updateSpec := specs["update-artifact"]
	updateSpec.Flags = []flagSpec{
		{Name: "name", Aliases: []string{"n"}, Value: "<name>", Description: "new artifact title", Inline: true},
	}
	configureTypedCommandSpecWithParseError(updateSpec,
		[]commandForm{{
			Parts: []operandSpec{withUsage(remainingOperand("arguments"), "<artifact-id> [title]")},
			Constraints: []constraint{
				constraintFunc(validateArtifactUpdateCommand),
			},
		}},
		decodeArtifactUpdate,
		func(_ string, err error) error {
			return fmt.Errorf("%w: %v", errBadArgs, err)
		},
	)
	configureTypedCommandSpec(specs["rename-artifact"],
		commandFormOf(
			requiredOperand("artifact"),
			withPlaceholder(requiredOperand("title"), "new-title"),
		),
		decodeArtifactRename,
	)
	configureTypedCommandSpec(specs["delete-artifact"], artifactForm, decodeArtifactDelete)
}

func validateArtifactExportCommand(parsed parsedCommand) error {
	_, err := decodeArtifactExportArgs(parsed)
	return err
}

func decodeArtifactExport(parsed parsedCommand) (commandCall, error) {
	args, err := decodeArtifactExportArgs(parsed)
	if err != nil {
		return nil, err
	}
	return func(_ context.Context, client *api.Client) error {
		return runArtifactExport(client, args)
	}, nil
}

func decodeArtifactExportArgs(parsed parsedCommand) (artifactExportArgs, error) {
	artifacts := parsed.Args["artifacts"]
	if len(artifacts) != 1 {
		return artifactExportArgs{}, fmt.Errorf("requires exactly one artifact id")
	}
	opts := artifactExportOptions{
		Format: parsedStringFlag(parsed, "format", "md"),
		Output: parsedStringFlag(parsed, "output", ""),
	}
	if err := normalizeArtifactExportOptions(&opts); err != nil {
		return artifactExportArgs{}, err
	}
	return artifactExportArgs{ArtifactID: artifacts[0], Options: opts}, nil
}

func validateArtifactUpdateCommand(parsed parsedCommand) error {
	_, err := decodeArtifactUpdateArgs(parsed)
	return err
}

func decodeArtifactUpdate(parsed parsedCommand) (commandCall, error) {
	args, err := decodeArtifactUpdateArgs(parsed)
	if err != nil {
		return nil, err
	}
	return func(_ context.Context, client *api.Client) error {
		return renameArtifact(client, args.ArtifactID, args.Options.Name)
	}, nil
}

func decodeArtifactUpdateArgs(parsed parsedCommand) (artifactUpdateArgs, error) {
	arguments := parsed.Args["arguments"]
	if len(arguments) < 1 || len(arguments) > 2 {
		return artifactUpdateArgs{}, fmt.Errorf("want artifact id and optional title")
	}
	name := parsedStringFlag(parsed, "name", parsed.globals.sourceName)
	if len(arguments) == 2 {
		name = arguments[1]
	}
	if name == "" {
		return artifactUpdateArgs{}, fmt.Errorf("provide new title as second arg or --name flag")
	}
	return artifactUpdateArgs{
		ArtifactID: arguments[0],
		Options:    updateArtifactOptions{Name: name},
	}, nil
}

func decodeArtifactGet(parsed parsedCommand) (commandCall, error) {
	args, err := decodeArtifactID(parsed)
	if err != nil {
		return nil, err
	}
	return func(_ context.Context, client *api.Client) error {
		return getArtifact(client, args.ArtifactID)
	}, nil
}

func decodeArtifactRead(parsed parsedCommand) (commandCall, error) {
	artifactID, err := parsedArgument(parsed, "artifact")
	if err != nil {
		return nil, err
	}
	args := artifactReadArgs{
		ArtifactID: artifactID,
		CDPURL:     parsedStringFlag(parsed, "cdp-url", parsed.globals.cdpURL),
	}
	return func(_ context.Context, client *api.Client) error {
		return readArtifact(client, args.ArtifactID, args.CDPURL)
	}, nil
}

func decodeArtifactList(parsed parsedCommand) (commandCall, error) {
	notebookID, err := parsedArgument(parsed, "notebook")
	if err != nil {
		return nil, err
	}
	jsonOutput, err := parsedBoolFlag(parsed, "json", parsed.globals.jsonOutput)
	if err != nil {
		return nil, err
	}
	args := artifactListArgs{NotebookID: notebookID, JSON: jsonOutput}
	return func(_ context.Context, client *api.Client) error {
		return listArtifacts(client, args.NotebookID, args.JSON)
	}, nil
}

func decodeArtifactRename(parsed parsedCommand) (commandCall, error) {
	artifactID, err := parsedArgument(parsed, "artifact")
	if err != nil {
		return nil, err
	}
	title, err := parsedArgument(parsed, "title")
	if err != nil {
		return nil, err
	}
	args := artifactRenameArgs{ArtifactID: artifactID, Title: title}
	return func(_ context.Context, client *api.Client) error {
		return renameArtifact(client, args.ArtifactID, args.Title)
	}, nil
}

func decodeArtifactDelete(parsed parsedCommand) (commandCall, error) {
	args, err := decodeArtifactID(parsed)
	if err != nil {
		return nil, err
	}
	args.Yes, err = parsedBoolFlag(parsed, "yes", parsed.globals.yes)
	if err != nil {
		return nil, err
	}
	return func(_ context.Context, client *api.Client) error {
		return deleteArtifact(client, args.ArtifactID, args.Yes)
	}, nil
}

func decodeArtifactID(parsed parsedCommand) (artifactIDArgs, error) {
	artifactID, err := parsedArgument(parsed, "artifact")
	if err != nil {
		return artifactIDArgs{}, err
	}
	return artifactIDArgs{ArtifactID: artifactID}, nil
}
