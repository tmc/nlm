package main

import (
	"context"

	"github.com/tmc/nlm/internal/notebooklm/api"
)

type artifactIDArgs struct {
	ArtifactID string
}

type artifactReadArgs struct {
	ArtifactID string
	Globals    globalOptions
}

type artifactListArgs struct {
	NotebookID string
}

type artifactRenameArgs struct {
	ArtifactID string
	Title      string
}

func configureArtifactCommandSpecs(specs map[commandID]*commandSpec) {
	artifactForm := commandFormOf(requiredOperand("artifact"))
	configureTypedCommandSpec(specs["get-artifact"], artifactForm, decodeArtifactGet)
	configureTypedCommandSpec(specs["read-artifact"], artifactForm, decodeArtifactRead)
	configureTypedCommandSpec(specs["artifacts"],
		commandFormOf(requiredOperand("notebook")),
		decodeArtifactList,
	)
	configureTypedCommandSpec(specs["rename-artifact"],
		commandFormOf(
			requiredOperand("artifact"),
			requiredOperand("title"),
		),
		decodeArtifactRename,
	)
	configureTypedCommandSpec(specs["delete-artifact"], artifactForm, decodeArtifactDelete)
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
		Globals:    parsed.globals,
	}
	return func(_ context.Context, client *api.Client) error {
		return readArtifact(client, args.ArtifactID, args.Globals)
	}, nil
}

func decodeArtifactList(parsed parsedCommand) (commandCall, error) {
	notebookID, err := parsedArgument(parsed, "notebook")
	if err != nil {
		return nil, err
	}
	args := artifactListArgs{NotebookID: notebookID}
	return func(_ context.Context, client *api.Client) error {
		return listArtifacts(client, args.NotebookID)
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
	return func(_ context.Context, client *api.Client) error {
		return deleteArtifact(client, args.ArtifactID)
	}, nil
}

func decodeArtifactID(parsed parsedCommand) (artifactIDArgs, error) {
	artifactID, err := parsedArgument(parsed, "artifact")
	if err != nil {
		return artifactIDArgs{}, err
	}
	return artifactIDArgs{ArtifactID: artifactID}, nil
}
