package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/tmc/nlm/internal/notebooklm/api"
)

type notebookCreateArgs struct {
	Title string
}

type notebookDeleteArgs struct {
	NotebookID string
	Yes        bool
}

type notebookRenameArgs struct {
	NotebookID string
	Title      string
}

type notebookEmojiArgs struct {
	NotebookID string
	Emoji      string
}

type notebookDescriptionArgs struct {
	NotebookID string
	Text       string
	TextSet    bool
}

type notebookCoverArgs struct {
	NotebookID string
	PresetID   string
}

type notebookCoverImageArgs struct {
	NotebookID string
	ImagePath  string
}

type notebookIDArgs struct {
	NotebookID string
}

type notebookFeaturedArgs struct {
	JSON bool
}

func configureNotebookCommandSpecs(specs map[commandID]*commandSpec) {
	listSpec := specs["list"]
	listSpec.Flags = []flagSpec{
		{Name: "all", Description: "show all notebooks when stdout is a terminal"},
		{Name: "limit", Value: "n", Description: "show at most n notebooks"},
		{Name: "json", Description: "emit NDJSON"},
	}
	configureTypedCommandSpecWithErrorUsage(listSpec,
		[]commandForm{{
			Parts: []operandSpec{hiddenOperand(remainingOperand("unexpected"))},
			Constraints: []constraint{
				constraintFunc(validateNotebookListCommand),
			},
		}},
		decodeNotebookList,
		func(path string, err error) {
			fmt.Fprintf(os.Stderr, "nlm: %v\n\n", err)
			printCommandHelpForPath(path)
		},
	)
	configureTypedCommandSpec(specs["create"],
		commandFormOf(requiredOperand("title")),
		decodeNotebookCreate,
	)
	configureTypedCommandSpec(specs["rm"],
		commandFormOf(requiredOperand("notebook")),
		decodeNotebookDelete,
	)
	configureTypedCommandSpec(specs["rename-notebook"],
		commandFormOf(requiredOperand("notebook"), withPlaceholder(requiredOperand("title"), "new-title")),
		decodeNotebookRename,
	)
	configureTypedCommandSpec(specs["notebook-emoji"],
		commandFormOf(requiredOperand("notebook"), requiredOperand("emoji")),
		decodeNotebookEmoji,
	)
	configureTypedCommandSpec(specs["notebook-description"],
		commandFormOf(requiredOperand("notebook"), optionalOperand("text")),
		decodeNotebookDescription,
	)
	configureTypedCommandSpec(specs["notebook-cover"],
		commandFormOf(requiredOperand("notebook"), requiredOperand("preset")),
		decodeNotebookCover,
	)
	configureTypedCommandSpec(specs["notebook-cover-image"],
		commandFormOf(requiredOperand("notebook"), requiredOperand("image")),
		decodeNotebookCoverImage,
	)
	configureTypedCommandSpec(specs["notebook-unrecent"],
		commandFormOf(requiredOperand("notebook")),
		decodeNotebookUnrecent,
	)
	configureTypedCommandSpec(specs["analytics"],
		commandFormOf(requiredOperand("notebook")),
		decodeNotebookAnalytics,
	)
	configureTypedCommandSpec(specs["list-featured"],
		commandFormOf(),
		decodeNotebookFeatured,
	)
}

func validateNotebookListCommand(parsed parsedCommand) error {
	_, err := decodeNotebookListOptions(parsed)
	return err
}

func decodeNotebookList(parsed parsedCommand) (commandCall, error) {
	args, err := decodeNotebookListOptions(parsed)
	if err != nil {
		return nil, err
	}
	return func(_ context.Context, client *api.Client) error {
		return list(client, args)
	}, nil
}

func decodeNotebookListOptions(parsed parsedCommand) (notebookListOptions, error) {
	opts := notebookListOptions{Limit: -1, JSON: parsed.globals.jsonOutput}
	if unexpected := parsed.Args["unexpected"]; len(unexpected) > 0 {
		return opts, fmt.Errorf("unexpected argument: %s", unexpected[0])
	}
	var err error
	opts.All, err = parsedBoolFlag(parsed, "all", false)
	if err != nil {
		return opts, err
	}
	opts.Limit, err = parsedIntFlag(parsed, "limit", -1)
	if err != nil {
		return opts, err
	}
	opts.JSON, err = parsedBoolFlag(parsed, "json", opts.JSON)
	if err != nil {
		return opts, err
	}
	if opts.Limit == 0 || opts.Limit < -1 {
		return opts, fmt.Errorf("--limit must be greater than 0")
	}
	if opts.All && opts.Limit > 0 {
		return opts, fmt.Errorf("--all and --limit cannot be used together")
	}
	return opts, nil
}

func decodeNotebookCreate(parsed parsedCommand) (commandCall, error) {
	title, err := parsedArgument(parsed, "title")
	if err != nil {
		return nil, err
	}
	args := notebookCreateArgs{Title: title}
	return func(_ context.Context, client *api.Client) error {
		return create(client, args.Title)
	}, nil
}

func decodeNotebookDelete(parsed parsedCommand) (commandCall, error) {
	notebookID, err := parsedArgument(parsed, "notebook")
	if err != nil {
		return nil, err
	}
	yes, err := parsedBoolFlag(parsed, "yes", parsed.globals.yes)
	if err != nil {
		return nil, err
	}
	args := notebookDeleteArgs{NotebookID: notebookID, Yes: yes}
	return func(_ context.Context, client *api.Client) error {
		return remove(client, args.NotebookID, args.Yes)
	}, nil
}

func decodeNotebookRename(parsed parsedCommand) (commandCall, error) {
	notebookID, err := parsedArgument(parsed, "notebook")
	if err != nil {
		return nil, err
	}
	title, err := parsedArgument(parsed, "title")
	if err != nil {
		return nil, err
	}
	args := notebookRenameArgs{NotebookID: notebookID, Title: title}
	return func(_ context.Context, client *api.Client) error {
		return renameNotebook(client, args.NotebookID, args.Title)
	}, nil
}

func decodeNotebookEmoji(parsed parsedCommand) (commandCall, error) {
	notebookID, err := parsedArgument(parsed, "notebook")
	if err != nil {
		return nil, err
	}
	emoji, err := parsedArgument(parsed, "emoji")
	if err != nil {
		return nil, err
	}
	args := notebookEmojiArgs{NotebookID: notebookID, Emoji: emoji}
	return func(_ context.Context, client *api.Client) error {
		return setNotebookEmoji(client, args.NotebookID, args.Emoji)
	}, nil
}

func decodeNotebookDescription(parsed parsedCommand) (commandCall, error) {
	notebookID, err := parsedArgument(parsed, "notebook")
	if err != nil {
		return nil, err
	}
	text, textSet, err := parsedOptionalArgument(parsed, "text")
	if err != nil {
		return nil, err
	}
	args := notebookDescriptionArgs{NotebookID: notebookID, Text: text, TextSet: textSet}
	return func(_ context.Context, client *api.Client) error {
		text := args.Text
		if !args.TextSet {
			if info, statErr := os.Stdin.Stat(); statErr == nil && info.Mode()&os.ModeCharDevice == 0 {
				data, readErr := io.ReadAll(os.Stdin)
				if readErr != nil {
					return readErr
				}
				text = string(data)
			}
		}
		return setNotebookDescription(client, args.NotebookID, text)
	}, nil
}

func decodeNotebookCover(parsed parsedCommand) (commandCall, error) {
	notebookID, err := parsedArgument(parsed, "notebook")
	if err != nil {
		return nil, err
	}
	presetID, err := parsedArgument(parsed, "preset")
	if err != nil {
		return nil, err
	}
	args := notebookCoverArgs{NotebookID: notebookID, PresetID: presetID}
	return func(_ context.Context, client *api.Client) error {
		id, err := strconv.Atoi(args.PresetID)
		if err != nil {
			return fmt.Errorf("preset-id must be an integer: %w", err)
		}
		return setNotebookCover(client, args.NotebookID, id)
	}, nil
}

func decodeNotebookCoverImage(parsed parsedCommand) (commandCall, error) {
	notebookID, err := parsedArgument(parsed, "notebook")
	if err != nil {
		return nil, err
	}
	imagePath, err := parsedArgument(parsed, "image")
	if err != nil {
		return nil, err
	}
	args := notebookCoverImageArgs{NotebookID: notebookID, ImagePath: imagePath}
	return func(_ context.Context, client *api.Client) error {
		return uploadNotebookCoverImage(client, args.NotebookID, args.ImagePath)
	}, nil
}

func decodeNotebookUnrecent(parsed parsedCommand) (commandCall, error) {
	notebookID, err := parsedArgument(parsed, "notebook")
	if err != nil {
		return nil, err
	}
	args := notebookIDArgs{NotebookID: notebookID}
	return func(ctx context.Context, client *api.Client) error {
		if err := client.RemoveRecentlyViewedProject(ctx, args.NotebookID); err != nil {
			return fmt.Errorf("remove recently viewed: %w", err)
		}
		fmt.Fprintf(os.Stderr, "Removed %s from recently viewed.\n", args.NotebookID)
		return nil
	}, nil
}

func decodeNotebookAnalytics(parsed parsedCommand) (commandCall, error) {
	notebookID, err := parsedArgument(parsed, "notebook")
	if err != nil {
		return nil, err
	}
	jsonOutput, err := parsedBoolFlag(parsed, "json", parsed.globals.jsonOutput)
	if err != nil {
		return nil, err
	}
	args := notebookIDArgs{NotebookID: notebookID}
	return func(_ context.Context, client *api.Client) error {
		return getAnalytics(client, args.NotebookID, jsonOutput)
	}, nil
}

func decodeNotebookFeatured(parsed parsedCommand) (commandCall, error) {
	jsonOutput, err := parsedBoolFlag(parsed, "json", parsed.globals.jsonOutput)
	if err != nil {
		return nil, err
	}
	args := notebookFeaturedArgs{JSON: jsonOutput}
	return func(_ context.Context, client *api.Client) error {
		return listFeaturedProjects(client, args.JSON)
	}, nil
}
