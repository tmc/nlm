package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/tmc/nlm/internal/notebooklm/api"
)

type noteListArgs struct {
	NotebookID string
}

type noteReadArgs struct {
	NotebookID string
	NoteID     string
	Options    noteReadOptions
}

type noteCreateArgs struct {
	NotebookID string
	Title      string
	Content    string
	ContentSet bool
}

type noteUpdateArgs struct {
	NotebookID string
	NoteID     string
	Content    string
	Title      string
}

type noteDeleteArgs struct {
	NotebookID string
	NoteID     string
}

func configureNoteCommandSpecs(specs map[commandID]*commandSpec) {
	configureTypedCommandSpec(specs["notes"],
		commandFormOf(requiredOperand("notebook")),
		decodeNoteList,
	)
	readSpec := specs["read-note"]
	readSpec.Flags = []flagSpec{
		{Name: "format", Value: "fmt", Description: "output format"},
		{Name: "out", Value: "file", Description: "html output file"},
		{Name: "open", Description: "open html output"},
	}
	configureTypedCommandSpecWithUsage(readSpec,
		[]commandForm{{
			Parts: []operandSpec{
				requiredOperand("notebook"),
				requiredOperand("note"),
			},
			Constraints: []constraint{
				constraintFunc(validateNoteReadCommand),
			},
		}},
		decodeNoteRead,
		func(path string) {
			fmt.Fprintf(os.Stderr, "usage: nlm %s [--format text|markdown|html] [--out file] [--open] <notebook-id> <note-id>\n", path)
		},
	)
	configureTypedCommandSpec(specs["new-note"],
		commandFormOf(
			requiredOperand("notebook"),
			requiredOperand("title"),
			optionalOperand("content"),
		),
		decodeNoteCreate,
	)
	configureTypedCommandSpec(specs["update-note"],
		commandFormOf(
			requiredOperand("notebook"),
			requiredOperand("note"),
			requiredOperand("content"),
			requiredOperand("title"),
		),
		decodeNoteUpdate,
	)
	configureTypedCommandSpec(specs["rm-note"],
		commandFormOf(requiredOperand("notebook"), requiredOperand("note")),
		decodeNoteDelete,
	)
}

func validateNoteReadCommand(parsed parsedCommand) error {
	_, err := decodeNoteReadArgs(parsed)
	return err
}

func decodeNoteRead(parsed parsedCommand) (commandCall, error) {
	args, err := decodeNoteReadArgs(parsed)
	if err != nil {
		return nil, err
	}
	return func(_ context.Context, client *api.Client) error {
		return readNoteWithOptions(client, args.NotebookID, args.NoteID, args.Options)
	}, nil
}

func decodeNoteReadArgs(parsed parsedCommand) (noteReadArgs, error) {
	notebookID, err := parsedArgument(parsed, "notebook")
	if err != nil {
		return noteReadArgs{}, err
	}
	noteID, err := parsedArgument(parsed, "note")
	if err != nil {
		return noteReadArgs{}, err
	}
	open, err := parsedBoolFlag(parsed, "open", false)
	if err != nil {
		return noteReadArgs{}, err
	}
	opts := noteReadOptions{
		Format:  parsedStringFlag(parsed, "format", ""),
		OutFile: parsedStringFlag(parsed, "out", ""),
		Open:    open,
	}
	if err := validateNoteFormat(&opts); err != nil {
		return noteReadArgs{}, err
	}
	return noteReadArgs{
		NotebookID: notebookID,
		NoteID:     noteID,
		Options:    opts,
	}, nil
}

func decodeNoteList(parsed parsedCommand) (commandCall, error) {
	notebookID, err := parsedArgument(parsed, "notebook")
	if err != nil {
		return nil, err
	}
	args := noteListArgs{NotebookID: notebookID}
	return func(_ context.Context, client *api.Client) error {
		return listNotes(client, args.NotebookID)
	}, nil
}

func decodeNoteCreate(parsed parsedCommand) (commandCall, error) {
	notebookID, err := parsedArgument(parsed, "notebook")
	if err != nil {
		return nil, err
	}
	title, err := parsedArgument(parsed, "title")
	if err != nil {
		return nil, err
	}
	content, contentSet, err := parsedOptionalArgument(parsed, "content")
	if err != nil {
		return nil, err
	}
	args := noteCreateArgs{
		NotebookID: notebookID,
		Title:      title,
		Content:    content,
		ContentSet: contentSet,
	}
	return func(_ context.Context, client *api.Client) error {
		content := args.Content
		if !args.ContentSet {
			if info, statErr := os.Stdin.Stat(); statErr == nil && info.Mode()&os.ModeCharDevice == 0 {
				data, readErr := io.ReadAll(os.Stdin)
				if readErr != nil {
					return readErr
				}
				content = string(data)
			}
		}
		return createNote(client, args.NotebookID, args.Title, content)
	}, nil
}

func decodeNoteUpdate(parsed parsedCommand) (commandCall, error) {
	notebookID, err := parsedArgument(parsed, "notebook")
	if err != nil {
		return nil, err
	}
	noteID, err := parsedArgument(parsed, "note")
	if err != nil {
		return nil, err
	}
	content, err := parsedArgument(parsed, "content")
	if err != nil {
		return nil, err
	}
	title, err := parsedArgument(parsed, "title")
	if err != nil {
		return nil, err
	}
	args := noteUpdateArgs{
		NotebookID: notebookID,
		NoteID:     noteID,
		Content:    content,
		Title:      title,
	}
	return func(_ context.Context, client *api.Client) error {
		return updateNote(client, args.NotebookID, args.NoteID, args.Content, args.Title)
	}, nil
}

func decodeNoteDelete(parsed parsedCommand) (commandCall, error) {
	notebookID, err := parsedArgument(parsed, "notebook")
	if err != nil {
		return nil, err
	}
	noteID, err := parsedArgument(parsed, "note")
	if err != nil {
		return nil, err
	}
	args := noteDeleteArgs{NotebookID: notebookID, NoteID: noteID}
	return func(_ context.Context, client *api.Client) error {
		return removeNote(client, args.NotebookID, args.NoteID)
	}, nil
}
