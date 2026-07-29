package main

import (
	"context"

	"github.com/tmc/nlm/notebooklm"
)

type sharingNotebookArgs struct {
	NotebookID string
}

type sharingDetailsArgs struct {
	ShareID string
}

func configureSharingCommandSpecs(specs map[commandID]*commandSpec) {
	notebookForm := commandFormOf(requiredOperand("notebook"))
	configureTypedCommandSpec(specs["share"], notebookForm, decodeSharePublic)
	configureTypedCommandSpec(specs["share-private"], notebookForm, decodeSharePrivate)
	configureTypedCommandSpec(specs["share-details"],
		commandFormOf(requiredOperand("share")),
		decodeShareDetails,
	)
}

func decodeSharePublic(parsed parsedCommand) (commandCall, error) {
	args, err := decodeSharingNotebook(parsed)
	if err != nil {
		return nil, err
	}
	return func(_ context.Context, client *notebooklm.Client) error {
		return shareNotebook(client, args.NotebookID)
	}, nil
}

func decodeSharePrivate(parsed parsedCommand) (commandCall, error) {
	args, err := decodeSharingNotebook(parsed)
	if err != nil {
		return nil, err
	}
	return func(_ context.Context, client *notebooklm.Client) error {
		return shareNotebookPrivate(client, args.NotebookID)
	}, nil
}

func decodeShareDetails(parsed parsedCommand) (commandCall, error) {
	shareID, err := parsedArgument(parsed, "share")
	if err != nil {
		return nil, err
	}
	args := sharingDetailsArgs{ShareID: shareID}
	return func(_ context.Context, client *notebooklm.Client) error {
		return getShareDetails(client, args.ShareID)
	}, nil
}

func decodeSharingNotebook(parsed parsedCommand) (sharingNotebookArgs, error) {
	notebookID, err := parsedArgument(parsed, "notebook")
	if err != nil {
		return sharingNotebookArgs{}, err
	}
	return sharingNotebookArgs{NotebookID: notebookID}, nil
}
