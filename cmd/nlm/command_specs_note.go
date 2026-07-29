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
	JSON       bool
}

type noteReadArgs struct {
	NotebookID string
	NoteID     string
	Options    noteReadOptions
}

type noteCreateArgs struct {
	NotebookID string
	Title      string
	Content    noteContentInput
	Grace      bool
}

type noteUpdateArgs struct {
	NotebookID string
	NoteID     string
	Title      *string
	Content    noteContentInput
	Full       bool
	Grace      bool
}

type noteDeleteArgs struct {
	NotebookID string
	NoteID     string
	Yes        bool
}

type noteContentInput struct {
	Text  *string
	File  *string
	Stdin bool
}

func configureNoteCommandSpecs(specs map[commandID]*commandSpec) {
	configureTypedCommandSpec(specs["notes"],
		commandFormOf(requiredOperand("notebook")),
		decodeNoteList,
	)
	readSpec := specs["read-note"]
	readSpec.Flags = []flagSpec{
		{Name: "format", Value: "text|markdown|html", Description: "output format", Inline: true},
		{Name: "out", Value: "file", Description: "html output file", Inline: true},
		{Name: "open", Description: "open html output", Inline: true},
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
			printCommandUsageForPath(path)
		},
	)
	createSpec := specs["new-note"]
	createSpec.FlagGroupAfter = 2
	configureTypedCommandSpec(createSpec,
		noteCreateForms(),
		decodeNoteCreate,
	)
	createSurface := findSpecSurface(createSpec, "note create")
	createSurface.Flags = noteContentFlagSpecs()
	findSpecSurface(createSpec, "new-note").Forms = legacyNoteCreateForms()

	updateSpec := specs["update-note"]
	updateSpec.FlagGroupAfter = 2
	configureTypedCommandSpec(updateSpec,
		noteUpdateForms(),
		decodeNoteUpdate,
	)
	updateSurface := findSpecSurface(updateSpec, "note update")
	updateSurface.Flags = append([]flagSpec{
		{Name: "title", Value: "TITLE", Description: "new note title", Inline: true},
	}, noteContentFlagSpecs()...)
	findSpecSurface(updateSpec, "update-note").Forms = legacyNoteUpdateForms()
	configureTypedCommandSpec(specs["rm-note"],
		commandFormOf(requiredOperand("notebook"), requiredOperand("note")),
		decodeNoteDelete,
	)
}

func noteContentFlagSpecs() []flagSpec {
	return []flagSpec{
		{
			Name:           "content",
			Value:          "TEXT",
			ExclusiveGroup: "content",
			Description:    "note content",
			Inline:         true,
		},
		{
			Name:           "content-file",
			Value:          "FILE",
			ExclusiveGroup: "content",
			Description:    "read note content from a file ('-' reads stdin)",
			Inline:         true,
		},
	}
}

func noteCreateForms() []commandForm {
	return []commandForm{
		{
			Parts: []operandSpec{
				requiredOperand("notebook"),
				requiredOperand("title"),
			},
			Constraints: []constraint{constraintFunc(validateNoteCreateCommand)},
		},
		{
			Parts: []operandSpec{
				requiredOperand("notebook"),
				requiredOperand("title"),
				requiredOperand("content"),
			},
			Constraints: []constraint{constraintFunc(validateNoteCreateCommand)},
			Hidden:      true,
		},
	}
}

func legacyNoteCreateForms() []commandForm {
	return commandFormOf(
		requiredOperand("notebook"),
		requiredOperand("title"),
		optionalOperand("content"),
	)
}

func noteUpdateForms() []commandForm {
	return []commandForm{
		{
			Parts: []operandSpec{
				requiredOperand("notebook"),
				requiredOperand("note"),
			},
			Constraints: []constraint{constraintFunc(validateNoteUpdateCommand)},
		},
		{
			Parts: []operandSpec{
				requiredOperand("notebook"),
				requiredOperand("note"),
				requiredOperand("content"),
				requiredOperand("title"),
			},
			Constraints: []constraint{constraintFunc(validateNoteUpdateCommand)},
			Hidden:      true,
		},
	}
}

func legacyNoteUpdateForms() []commandForm {
	return commandFormOf(
		requiredOperand("notebook"),
		requiredOperand("note"),
		requiredOperand("content"),
		requiredOperand("title"),
	)
}

func validateNoteReadCommand(parsed parsedCommand) error {
	_, err := decodeNoteReadArgs(parsed)
	return err
}

func validateNoteCreateCommand(parsed parsedCommand) error {
	_, err := decodeNoteCreateArgs(parsed)
	return err
}

func validateNoteUpdateCommand(parsed parsedCommand) error {
	_, err := decodeNoteUpdateArgs(parsed)
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
	jsonOutput, err := parsedBoolFlag(parsed, "json", parsed.globals.jsonOutput)
	if err != nil {
		return nil, err
	}
	args := noteListArgs{NotebookID: notebookID, JSON: jsonOutput}
	return func(_ context.Context, client *api.Client) error {
		return listNotes(client, args.NotebookID, args.JSON)
	}, nil
}

func decodeNoteCreate(parsed parsedCommand) (commandCall, error) {
	args, err := decodeNoteCreateArgs(parsed)
	if err != nil {
		return nil, err
	}
	return func(_ context.Context, client *api.Client) error {
		return runNoteCreate(os.Stderr, os.Stdin, args, func(title, content string) error {
			return createNote(client, args.NotebookID, title, content)
		})
	}, nil
}

func decodeNoteCreateArgs(parsed parsedCommand) (noteCreateArgs, error) {
	return decodeNoteCreateArgsWithStdin(parsed, noteStdinIsPiped())
}

func decodeNoteCreateArgsWithStdin(parsed parsedCommand, stdinPiped bool) (noteCreateArgs, error) {
	notebookID, err := parsedArgument(parsed, "notebook")
	if err != nil {
		return noteCreateArgs{}, err
	}
	title, err := parsedArgument(parsed, "title")
	if err != nil {
		return noteCreateArgs{}, err
	}
	content, positionalContent, err := parsedOptionalArgument(parsed, "content")
	if err != nil {
		return noteCreateArgs{}, err
	}
	args := noteCreateArgs{NotebookID: notebookID, Title: title}
	if parsed.path != "note create" {
		if positionalContent {
			args.Content.Text = &content
		} else if stdinPiped {
			args.Content.Stdin = true
		}
		return args, nil
	}

	flagContent, flagContentSet := parsedNoteFlag(parsed, "content")
	contentFile, contentFileSet := parsedNoteFlag(parsed, "content-file")
	implicitStdin := stdinPiped && !(contentFileSet && contentFile == "-")
	contentSources := countTrue(positionalContent, flagContentSet, contentFileSet, implicitStdin)
	if contentSources > 1 {
		return noteCreateArgs{}, fmt.Errorf("choose only one of positional content, --content, --content-file, or piped stdin")
	}
	switch {
	case positionalContent:
		args.Content.Text = &content
		args.Grace = true
	case flagContentSet:
		args.Content.Text = &flagContent
	case contentFileSet:
		args.Content.File = &contentFile
	case implicitStdin:
		args.Content.Stdin = true
	}
	return args, nil
}

func decodeNoteUpdate(parsed parsedCommand) (commandCall, error) {
	args, err := decodeNoteUpdateArgs(parsed)
	if err != nil {
		return nil, err
	}
	return func(_ context.Context, client *api.Client) error {
		return runNoteUpdate(
			os.Stderr,
			os.Stdin,
			args,
			func(content, title string) error {
				return updateNote(client, args.NotebookID, args.NoteID, content, title)
			},
			func(title, content *string) error {
				return updateNoteFields(client, args.NotebookID, args.NoteID, title, content)
			},
		)
	}, nil
}

func decodeNoteUpdateArgs(parsed parsedCommand) (noteUpdateArgs, error) {
	return decodeNoteUpdateArgsWithStdin(parsed, noteStdinIsPiped())
}

func decodeNoteUpdateArgsWithStdin(parsed parsedCommand, stdinPiped bool) (noteUpdateArgs, error) {
	notebookID, err := parsedArgument(parsed, "notebook")
	if err != nil {
		return noteUpdateArgs{}, err
	}
	noteID, err := parsedArgument(parsed, "note")
	if err != nil {
		return noteUpdateArgs{}, err
	}
	content, positionalContent, err := parsedOptionalArgument(parsed, "content")
	if err != nil {
		return noteUpdateArgs{}, err
	}
	title, positionalTitle, err := parsedOptionalArgument(parsed, "title")
	if err != nil {
		return noteUpdateArgs{}, err
	}
	args := noteUpdateArgs{NotebookID: notebookID, NoteID: noteID}
	if parsed.path != "note update" {
		args.Title = &title
		args.Content.Text = &content
		args.Full = true
		return args, nil
	}

	flagTitle, flagTitleSet := parsedNoteFlag(parsed, "title")
	flagContent, flagContentSet := parsedNoteFlag(parsed, "content")
	contentFile, contentFileSet := parsedNoteFlag(parsed, "content-file")
	implicitStdin := stdinPiped && !(contentFileSet && contentFile == "-")
	if positionalContent || positionalTitle {
		if !positionalContent || !positionalTitle {
			return noteUpdateArgs{}, fmt.Errorf("deprecated positional update requires content and title")
		}
		if countTrue(flagTitleSet, flagContentSet, contentFileSet, implicitStdin) != 0 {
			return noteUpdateArgs{}, fmt.Errorf("deprecated positional update cannot be combined with named fields or piped stdin")
		}
		args.Title = &title
		args.Content.Text = &content
		args.Full = true
		args.Grace = true
		return args, nil
	}
	if flagTitleSet {
		if flagTitle == "" {
			return noteUpdateArgs{}, fmt.Errorf("--title cannot be empty")
		}
		args.Title = &flagTitle
	}
	if countTrue(flagContentSet, contentFileSet, implicitStdin) > 1 {
		return noteUpdateArgs{}, fmt.Errorf("choose only one of --content, --content-file, or piped stdin")
	}
	switch {
	case flagContentSet:
		args.Content.Text = &flagContent
	case contentFileSet:
		args.Content.File = &contentFile
	case implicitStdin:
		args.Content.Stdin = true
	}
	if args.Title == nil && !args.Content.set() {
		return noteUpdateArgs{}, fmt.Errorf("provide --title, --content, --content-file, or piped stdin")
	}
	return args, nil
}

func parsedNoteFlag(parsed parsedCommand, name string) (string, bool) {
	values := parsed.Flags[name]
	if len(values) == 0 {
		return "", false
	}
	return values[len(values)-1], true
}

func countTrue(values ...bool) int {
	count := 0
	for _, value := range values {
		if value {
			count++
		}
	}
	return count
}

func (input noteContentInput) set() bool {
	return input.Text != nil || input.File != nil || input.Stdin
}

func noteStdinIsPiped() bool {
	info, err := os.Stdin.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice == 0
}

func readNoteContent(input noteContentInput, stdin io.Reader) (*string, error) {
	if input.Text != nil {
		return input.Text, nil
	}
	if input.File != nil {
		var data []byte
		var err error
		if *input.File == "-" {
			data, err = io.ReadAll(stdin)
		} else {
			data, err = os.ReadFile(*input.File)
		}
		if err != nil {
			return nil, fmt.Errorf("read note content from %q: %w", *input.File, err)
		}
		content := string(data)
		return &content, nil
	}
	if input.Stdin {
		data, err := io.ReadAll(stdin)
		if err != nil {
			return nil, fmt.Errorf("read note content from stdin: %w", err)
		}
		content := string(data)
		return &content, nil
	}
	return nil, nil
}

func runNoteCreate(
	stderr io.Writer,
	stdin io.Reader,
	args noteCreateArgs,
	create func(title, content string) error,
) error {
	if args.Grace {
		warnDeprecatedNoteCreate(stderr)
	}
	content, err := readNoteContent(args.Content, stdin)
	if err != nil {
		return err
	}
	if content == nil {
		empty := ""
		content = &empty
	}
	return create(args.Title, *content)
}

func runNoteUpdate(
	stderr io.Writer,
	stdin io.Reader,
	args noteUpdateArgs,
	full func(content, title string) error,
	partial func(title, content *string) error,
) error {
	if args.Grace {
		warnDeprecatedNoteUpdate(stderr)
	}
	content, err := readNoteContent(args.Content, stdin)
	if err != nil {
		return err
	}
	if args.Full {
		return full(*content, *args.Title)
	}
	return partial(args.Title, content)
}

func warnDeprecatedNoteCreate(w io.Writer) {
	fmt.Fprintln(w, "nlm: 'note create <notebook-id> <title> <content>' is deprecated; use 'note create <notebook-id> <title> --content <content>'")
}

func warnDeprecatedNoteUpdate(w io.Writer) {
	fmt.Fprintln(w, "nlm: 'note update <notebook-id> <note-id> <content> <title>' uses deprecated positional mutation fields; use 'note update <notebook-id> <note-id> --title <title> --content <content>'")
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
	yes, err := parsedBoolFlag(parsed, "yes", parsed.globals.yes)
	if err != nil {
		return nil, err
	}
	args := noteDeleteArgs{NotebookID: notebookID, NoteID: noteID, Yes: yes}
	return func(_ context.Context, client *api.Client) error {
		return removeNote(client, args.NotebookID, args.NoteID, args.Yes)
	}, nil
}
