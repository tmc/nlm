package main

import (
	"context"
	"fmt"
	"os"

	"github.com/tmc/nlm/internal/notebooklm/api"
)

type sourceListArgs struct {
	NotebookID string
}

type sourceDeleteArgs struct {
	NotebookID string
	SourceIDs  string
}

type sourceRenameArgs struct {
	SourceID string
	Name     string
}

type sourceRefreshArgs struct {
	NotebookID string
	SourceID   string
}

type sourceCheckArgs struct {
	SourceID   string
	NotebookID string
}

type sourceDiscoverArgs struct {
	NotebookID string
	Query      string
	Globals    globalOptions
}

type sourceDumpArgs struct {
	SourceID   string
	NotebookID string
}

func configureSourceCommandSpecs(specs map[commandID]*commandSpec) {
	configureTypedCommandSpec(specs["sources"],
		commandFormOf(requiredOperand("notebook")),
		decodeSourceList,
	)
	configureTypedCommandSpec(specs["rm-source"],
		commandFormOf(requiredOperand("notebook"), requiredOperand("sources")),
		decodeSourceDelete,
	)
	configureTypedCommandSpec(specs["rename-source"],
		commandFormOf(requiredOperand("source"), requiredOperand("name")),
		decodeSourceRename,
	)
	configureTypedCommandSpec(specs["refresh-source"],
		commandFormOf(requiredOperand("notebook"), requiredOperand("source")),
		decodeSourceRefresh,
	)
	configureTypedCommandSpec(specs["check-source"],
		commandFormOf(requiredOperand("source"), optionalOperand("notebook")),
		decodeSourceCheck,
	)
	configureTypedCommandSpec(specs["discover-sources"],
		commandFormOf(requiredOperand("notebook"), requiredOperand("query")),
		decodeSourceDiscover,
	)
	configureTypedCommandSpec(specs["dump-load-source"],
		commandFormOf(requiredOperand("source"), optionalOperand("notebook")),
		decodeSourceDump,
	)
}

func decodeSourceList(parsed parsedCommand) (commandCall, error) {
	notebookID, err := parsedArgument(parsed, "notebook")
	if err != nil {
		return nil, err
	}
	args := sourceListArgs{NotebookID: notebookID}
	return func(_ context.Context, client *api.Client) error {
		return listSources(client, args.NotebookID)
	}, nil
}

func decodeSourceDelete(parsed parsedCommand) (commandCall, error) {
	notebookID, err := parsedArgument(parsed, "notebook")
	if err != nil {
		return nil, err
	}
	sourceIDs, err := parsedArgument(parsed, "sources")
	if err != nil {
		return nil, err
	}
	args := sourceDeleteArgs{NotebookID: notebookID, SourceIDs: sourceIDs}
	return func(ctx context.Context, client *api.Client) error {
		return removeSource(ctx, client, args.NotebookID, args.SourceIDs)
	}, nil
}

func decodeSourceRename(parsed parsedCommand) (commandCall, error) {
	sourceID, err := parsedArgument(parsed, "source")
	if err != nil {
		return nil, err
	}
	name, err := parsedArgument(parsed, "name")
	if err != nil {
		return nil, err
	}
	args := sourceRenameArgs{SourceID: sourceID, Name: name}
	return func(_ context.Context, client *api.Client) error {
		return renameSource(client, args.SourceID, args.Name)
	}, nil
}

func decodeSourceRefresh(parsed parsedCommand) (commandCall, error) {
	notebookID, err := parsedArgument(parsed, "notebook")
	if err != nil {
		return nil, err
	}
	sourceID, err := parsedArgument(parsed, "source")
	if err != nil {
		return nil, err
	}
	args := sourceRefreshArgs{NotebookID: notebookID, SourceID: sourceID}
	return func(_ context.Context, client *api.Client) error {
		return refreshSource(client, args.NotebookID, args.SourceID)
	}, nil
}

func decodeSourceCheck(parsed parsedCommand) (commandCall, error) {
	sourceID, err := parsedArgument(parsed, "source")
	if err != nil {
		return nil, err
	}
	notebookID, _, err := parsedOptionalArgument(parsed, "notebook")
	if err != nil {
		return nil, err
	}
	args := sourceCheckArgs{SourceID: sourceID, NotebookID: notebookID}
	return func(_ context.Context, client *api.Client) error {
		return checkSourceFreshness(client, args.SourceID, args.NotebookID)
	}, nil
}

func decodeSourceDiscover(parsed parsedCommand) (commandCall, error) {
	notebookID, err := parsedArgument(parsed, "notebook")
	if err != nil {
		return nil, err
	}
	query, err := parsedArgument(parsed, "query")
	if err != nil {
		return nil, err
	}
	args := sourceDiscoverArgs{
		NotebookID: notebookID,
		Query:      query,
		Globals:    parsed.globals,
	}
	return func(_ context.Context, client *api.Client) error {
		return discoverSources(client, args.NotebookID, args.Query, args.Globals)
	}, nil
}

func decodeSourceDump(parsed parsedCommand) (commandCall, error) {
	sourceID, err := parsedArgument(parsed, "source")
	if err != nil {
		return nil, err
	}
	notebookID, _, err := parsedOptionalArgument(parsed, "notebook")
	if err != nil {
		return nil, err
	}
	args := sourceDumpArgs{SourceID: sourceID, NotebookID: notebookID}
	return func(ctx context.Context, client *api.Client) error {
		raw, err := client.LoadSourceRaw(ctx, args.SourceID, args.NotebookID)
		if err != nil {
			return err
		}
		if _, err := os.Stdout.Write(raw); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(os.Stdout); err != nil {
			return err
		}
		return nil
	}, nil
}
