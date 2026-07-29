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

type notebookFeaturedArgs struct{}

func configureNotebookCommandSpecs(specs map[commandID]*commandSpec) {
	configureTypedCommandSpec(specs["create"],
		commandFormOf(requiredOperand("title")),
		decodeNotebookCreate,
	)
	configureTypedCommandSpec(specs["rm"],
		commandFormOf(requiredOperand("notebook")),
		decodeNotebookDelete,
	)
	configureTypedCommandSpec(specs["rename-notebook"],
		commandFormOf(requiredOperand("notebook"), requiredOperand("title")),
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
	args := notebookDeleteArgs{NotebookID: notebookID}
	return func(_ context.Context, client *api.Client) error {
		return remove(client, args.NotebookID)
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
	args := notebookIDArgs{NotebookID: notebookID}
	return func(_ context.Context, client *api.Client) error {
		return getAnalytics(client, args.NotebookID)
	}, nil
}

func decodeNotebookFeatured(parsedCommand) (commandCall, error) {
	args := notebookFeaturedArgs{}
	return func(_ context.Context, client *api.Client) error {
		_ = args
		return listFeaturedProjects(client)
	}, nil
}
