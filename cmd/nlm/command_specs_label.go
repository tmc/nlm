package main

import (
	"context"
	"fmt"
	"os"

	"github.com/tmc/nlm/internal/notebooklm/api"
)

type labelNotebookArgs struct {
	NotebookID string
	JSON       bool
}

type labelCreateArgs struct {
	NotebookID string
	Name       string
	Emoji      string
	JSON       bool
}

type labelRenameArgs struct {
	NotebookID string
	LabelID    string
	Name       string
}

type labelEmojiArgs struct {
	NotebookID string
	LabelID    string
	Emoji      string
}

type labelDeleteArgs struct {
	NotebookID string
	LabelIDs   []string
}

type labelAttachArgs struct {
	NotebookID string
	Label      string
	Source     string
}

func configureLabelCommandSpecs(specs map[commandID]*commandSpec) {
	configureTypedCommandSpecWithUsage(specs["label-list"],
		commandFormOf(requiredOperand("notebook")),
		decodeLabelList,
		func(path string) {
			fmt.Fprintf(os.Stderr, "nlm: %s requires exactly one argument: <notebook-id>\n\n", path)
			printCommandHelpForPath(path)
		},
	)
	configureTypedCommandSpecWithUsage(specs["label-generate"],
		commandFormOf(requiredOperand("notebook")),
		decodeLabelGenerate,
		func(path string) {
			fmt.Fprintf(os.Stderr, "nlm: %s requires exactly one argument: <notebook-id>\n\n", path)
			printCommandHelpForPath(path)
		},
	)
	configureTypedCommandSpecWithUsage(specs["label-create"],
		commandFormOf(
			requiredOperand("notebook"),
			requiredOperand("name"),
			optionalOperand("emoji"),
		),
		decodeLabelCreate,
		func(path string) {
			fmt.Fprintf(os.Stderr, "nlm: %s requires <notebook-id> <name> [emoji]\n\n", path)
			printCommandHelpForPath(path)
		},
	)
	configureTypedCommandSpecWithUsage(specs["label-rename"],
		commandFormOf(
			requiredOperand("notebook"),
			requiredOperand("label"),
			withPlaceholder(requiredOperand("name"), "new-name"),
		),
		decodeLabelRename,
		func(path string) {
			fmt.Fprintf(os.Stderr, "nlm: %s requires <notebook-id> <label-id> <new-name>\n\n", path)
			printCommandHelpForPath(path)
		},
	)
	configureTypedCommandSpecWithUsage(specs["label-emoji"],
		commandFormOf(
			requiredOperand("notebook"),
			requiredOperand("label"),
			requiredOperand("emoji"),
		),
		decodeLabelEmoji,
		func(path string) {
			fmt.Fprintf(os.Stderr, "nlm: %s requires <notebook-id> <label-id> <emoji>\n\n", path)
			printCommandHelpForPath(path)
		},
	)
	configureTypedCommandSpecWithUsage(specs["label-delete"],
		commandFormOf(
			requiredOperand("notebook"),
			withUsage(repeatedOperand("labels"), "<label-id> [<label-id>...]"),
		),
		decodeLabelDelete,
		func(path string) {
			fmt.Fprintf(os.Stderr, "nlm: %s requires <notebook-id> and at least one <label-id>\n\n", path)
			printCommandHelpForPath(path)
		},
	)
	configureTypedCommandSpecWithUsage(specs["label-unlabeled"],
		commandFormOf(requiredOperand("notebook")),
		decodeLabelUnlabeled,
		func(path string) {
			fmt.Fprintf(os.Stderr, "nlm: %s requires exactly one argument: <notebook-id>\n\n", path)
			printCommandHelpForPath(path)
		},
	)
	configureTypedCommandSpecWithUsage(specs["label-relabel-all"],
		commandFormOf(requiredOperand("notebook")),
		decodeLabelRelabelAll,
		func(path string) {
			fmt.Fprintf(os.Stderr, "nlm: %s requires exactly one argument: <notebook-id>\n\n", path)
			printCommandHelpForPath(path)
		},
	)
	configureTypedCommandSpecWithUsage(specs["label-attach"],
		commandFormOf(
			requiredOperand("notebook"),
			withPlaceholder(requiredOperand("label"), "label-id|name"),
			withPlaceholder(requiredOperand("source"), "source-id|name"),
		),
		decodeLabelAttach,
		func(path string) {
			fmt.Fprintf(os.Stderr, "nlm: %s requires <notebook-id> <label-id|name> <source-id|name>\n\n", path)
			printCommandHelpForPath(path)
		},
	)
}

func decodeLabelList(parsed parsedCommand) (commandCall, error) {
	args, err := decodeLabelNotebook(parsed)
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context, client *api.Client) error {
		labels, err := client.GetLabels(ctx, args.NotebookID)
		if err != nil {
			return err
		}
		return renderLabelList(os.Stdout, os.Stderr, labels, isTerminal(os.Stdout), args.JSON)
	}, nil
}

func decodeLabelGenerate(parsed parsedCommand) (commandCall, error) {
	args, err := decodeLabelNotebook(parsed)
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context, client *api.Client) error {
		labels, err := client.GenerateLabels(ctx, args.NotebookID)
		if err != nil {
			return err
		}
		return renderLabelList(os.Stdout, os.Stderr, labels, isTerminal(os.Stdout), args.JSON)
	}, nil
}

func decodeLabelCreate(parsed parsedCommand) (commandCall, error) {
	notebookID, err := parsedArgument(parsed, "notebook")
	if err != nil {
		return nil, err
	}
	name, err := parsedArgument(parsed, "name")
	if err != nil {
		return nil, err
	}
	emoji, _, err := parsedOptionalArgument(parsed, "emoji")
	if err != nil {
		return nil, err
	}
	jsonOutput, err := parsedBoolFlag(parsed, "json", parsed.globals.jsonOutput)
	if err != nil {
		return nil, err
	}
	args := labelCreateArgs{NotebookID: notebookID, Name: name, Emoji: emoji, JSON: jsonOutput}
	return func(ctx context.Context, client *api.Client) error {
		labels, err := client.CreateLabel(ctx, args.NotebookID, args.Name, args.Emoji)
		if err != nil {
			return err
		}
		return renderLabelList(os.Stdout, os.Stderr, labels, isTerminal(os.Stdout), args.JSON)
	}, nil
}

func decodeLabelRename(parsed parsedCommand) (commandCall, error) {
	notebookID, err := parsedArgument(parsed, "notebook")
	if err != nil {
		return nil, err
	}
	labelID, err := parsedArgument(parsed, "label")
	if err != nil {
		return nil, err
	}
	name, err := parsedArgument(parsed, "name")
	if err != nil {
		return nil, err
	}
	args := labelRenameArgs{NotebookID: notebookID, LabelID: labelID, Name: name}
	return func(ctx context.Context, client *api.Client) error {
		if err := client.RenameLabel(ctx, args.NotebookID, args.LabelID, args.Name); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "Renamed %s to %q\n", args.LabelID, args.Name)
		return nil
	}, nil
}

func decodeLabelEmoji(parsed parsedCommand) (commandCall, error) {
	notebookID, err := parsedArgument(parsed, "notebook")
	if err != nil {
		return nil, err
	}
	labelID, err := parsedArgument(parsed, "label")
	if err != nil {
		return nil, err
	}
	emoji, err := parsedArgument(parsed, "emoji")
	if err != nil {
		return nil, err
	}
	args := labelEmojiArgs{NotebookID: notebookID, LabelID: labelID, Emoji: emoji}
	return func(ctx context.Context, client *api.Client) error {
		if err := client.SetLabelEmoji(ctx, args.NotebookID, args.LabelID, args.Emoji); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "Set emoji on %s\n", args.LabelID)
		return nil
	}, nil
}

func decodeLabelDelete(parsed parsedCommand) (commandCall, error) {
	notebookID, err := parsedArgument(parsed, "notebook")
	if err != nil {
		return nil, err
	}
	labelIDs, err := parsedArguments(parsed, "labels")
	if err != nil {
		return nil, err
	}
	args := labelDeleteArgs{NotebookID: notebookID, LabelIDs: labelIDs}
	return func(ctx context.Context, client *api.Client) error {
		if err := client.DeleteLabels(ctx, args.NotebookID, args.LabelIDs); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "Deleted %d label(s)\n", len(args.LabelIDs))
		return nil
	}, nil
}

func decodeLabelUnlabeled(parsed parsedCommand) (commandCall, error) {
	args, err := decodeLabelNotebook(parsed)
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context, client *api.Client) error {
		labels, err := client.LabelUnlabeled(ctx, args.NotebookID)
		if err != nil {
			return err
		}
		return renderLabelList(os.Stdout, os.Stderr, labels, isTerminal(os.Stdout), args.JSON)
	}, nil
}

func decodeLabelRelabelAll(parsed parsedCommand) (commandCall, error) {
	args, err := decodeLabelNotebook(parsed)
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context, client *api.Client) error {
		labels, err := client.RelabelAll(ctx, args.NotebookID)
		if err != nil {
			return err
		}
		return renderLabelList(os.Stdout, os.Stderr, labels, isTerminal(os.Stdout), args.JSON)
	}, nil
}

func decodeLabelAttach(parsed parsedCommand) (commandCall, error) {
	notebookID, err := parsedArgument(parsed, "notebook")
	if err != nil {
		return nil, err
	}
	label, err := parsedArgument(parsed, "label")
	if err != nil {
		return nil, err
	}
	source, err := parsedArgument(parsed, "source")
	if err != nil {
		return nil, err
	}
	args := labelAttachArgs{NotebookID: notebookID, Label: label, Source: source}
	return func(ctx context.Context, client *api.Client) error {
		labelID, err := resolveLabelArg(client, args.NotebookID, args.Label)
		if err != nil {
			return err
		}
		sourceID, err := resolveSourceArg(client, args.NotebookID, args.Source)
		if err != nil {
			return err
		}
		if err := client.AttachLabelSource(ctx, args.NotebookID, labelID, sourceID); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "Attached source %s to label %s\n", sourceID, labelID)
		return nil
	}, nil
}

func decodeLabelNotebook(parsed parsedCommand) (labelNotebookArgs, error) {
	notebookID, err := parsedArgument(parsed, "notebook")
	if err != nil {
		return labelNotebookArgs{}, err
	}
	jsonOutput, err := parsedBoolFlag(parsed, "json", parsed.globals.jsonOutput)
	if err != nil {
		return labelNotebookArgs{}, err
	}
	return labelNotebookArgs{NotebookID: notebookID, JSON: jsonOutput}, nil
}
