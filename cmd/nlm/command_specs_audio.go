package main

import (
	"context"

	"github.com/tmc/nlm/notebooklm"
)

type audioNotebookArgs struct {
	NotebookID string
	JSON       bool
	Yes        bool
}

type audioDownloadArgs struct {
	NotebookID string
	Filename   string
}

func configureAudioCommandSpecs(specs map[commandID]*commandSpec) {
	notebookForm := commandFormOf(requiredOperand("notebook"))
	configureTypedCommandSpec(specs["audio-list"], notebookForm, decodeAudioList)
	configureTypedCommandSpec(specs["audio-get"], notebookForm, decodeAudioGet)
	configureTypedCommandSpec(specs["audio-download"],
		commandFormOf(
			requiredOperand("notebook"),
			optionalOperand("filename"),
		),
		decodeAudioDownload,
	)
	configureTypedCommandSpec(specs["audio-rm"], notebookForm, decodeAudioDelete)
	configureTypedCommandSpec(specs["audio-share"], notebookForm, decodeAudioShare)
}

func decodeAudioList(parsed parsedCommand) (commandCall, error) {
	args, err := decodeAudioNotebook(parsed)
	if err != nil {
		return nil, err
	}
	return func(_ context.Context, client *notebooklm.Client) error {
		return listAudioOverviews(client, args.NotebookID, args.JSON)
	}, nil
}

func decodeAudioGet(parsed parsedCommand) (commandCall, error) {
	args, err := decodeAudioNotebook(parsed)
	if err != nil {
		return nil, err
	}
	return func(_ context.Context, client *notebooklm.Client) error {
		return getAudioOverview(client, args.NotebookID)
	}, nil
}

func decodeAudioDownload(parsed parsedCommand) (commandCall, error) {
	notebookID, err := parsedArgument(parsed, "notebook")
	if err != nil {
		return nil, err
	}
	filename, _, err := parsedOptionalArgument(parsed, "filename")
	if err != nil {
		return nil, err
	}
	args := audioDownloadArgs{NotebookID: notebookID, Filename: filename}
	return func(_ context.Context, client *notebooklm.Client) error {
		return downloadAudioOverview(client, args.NotebookID, args.Filename)
	}, nil
}

func decodeAudioDelete(parsed parsedCommand) (commandCall, error) {
	args, err := decodeAudioNotebook(parsed)
	if err != nil {
		return nil, err
	}
	return func(_ context.Context, client *notebooklm.Client) error {
		return deleteAudioOverview(client, args.NotebookID, args.Yes)
	}, nil
}

func decodeAudioShare(parsed parsedCommand) (commandCall, error) {
	args, err := decodeAudioNotebook(parsed)
	if err != nil {
		return nil, err
	}
	return func(_ context.Context, client *notebooklm.Client) error {
		return shareAudioOverview(client, args.NotebookID)
	}, nil
}

func decodeAudioNotebook(parsed parsedCommand) (audioNotebookArgs, error) {
	notebookID, err := parsedArgument(parsed, "notebook")
	if err != nil {
		return audioNotebookArgs{}, err
	}
	jsonOutput, err := parsedBoolFlag(parsed, "json", parsed.globals.jsonOutput)
	if err != nil {
		return audioNotebookArgs{}, err
	}
	yes, err := parsedBoolFlag(parsed, "yes", parsed.globals.yes)
	if err != nil {
		return audioNotebookArgs{}, err
	}
	return audioNotebookArgs{NotebookID: notebookID, JSON: jsonOutput, Yes: yes}, nil
}
