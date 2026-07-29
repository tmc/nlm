package main

import (
	"context"
	"fmt"
	"os"

	"github.com/tmc/nlm/internal/nlmsync"
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

type sourceReadArgs struct {
	SourceID   string
	NotebookID string
	Options    globalOptions
	Warnings   globalOptions
}

type sourceAddArgs struct {
	NotebookID string
	Inputs     []string
	Options    sourceAddOptions
}

type sourceSyncArgs struct {
	NotebookID string
	Paths      []string
	Options    syncOptions
}

type sourcePackArgs struct {
	Paths   []string
	Options syncPackOptions
}

func configureSourceCommandSpecs(specs map[commandID]*commandSpec) {
	configureTypedCommandSpec(specs["sources"],
		commandFormOf(requiredOperand("notebook")),
		decodeSourceList,
	)
	configureSourceAddSpec(specs["add"])
	configureSourceSyncSpec(specs["sync"])
	configureSourcePackSpec(specs["sync-pack"])
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
	readSpec := specs["read-source"]
	readSpec.Flags = []flagSpec{
		{Name: "format", Value: "format", Description: "output format"},
	}
	configureTypedCommandSpecWithUsage(readSpec,
		[]commandForm{{
			Parts: []operandSpec{
				requiredOperand("source"),
				optionalOperand("notebook"),
			},
			Constraints: []constraint{
				constraintFunc(validateSourceReadCommand),
			},
		}},
		decodeSourceRead,
		func(path string) {
			fmt.Fprintf(os.Stderr, "usage: nlm %s [--format text|markdown|html|json|raw] <source-id> [notebook-id]\n", path)
		},
	)
}

func configureSourceAddSpec(spec *commandSpec) {
	spec.Flags = []flagSpec{
		{Name: "name", Aliases: []string{"n"}, Value: "name", Description: "source name"},
		{Name: "mime", Aliases: []string{"mime-type"}, Value: "type", Description: "MIME type"},
		{Name: "replace", Value: "source-id", Description: "source to replace"},
		{Name: "pre-process", Value: "command", Description: "pre-process command"},
		{Name: "chunk", Value: "bytes", Description: "chunk size"},
	}
	configureTypedCommandSpecWithUsage(spec,
		[]commandForm{{
			Parts: []operandSpec{remainingOperand("positionals")},
			Constraints: []constraint{
				constraintFunc(validateSourceAddCommand),
			},
		}},
		decodeSourceAdd,
		func(path string) {
			fmt.Fprintf(os.Stderr, "usage: nlm %s <notebook-id> <source|-> [source...]\n", path)
		},
	)
}

func configureSourceSyncSpec(spec *commandSpec) {
	spec.Flags = []flagSpec{
		{Name: "name", Aliases: []string{"n"}, Value: "name", Description: "source name"},
		{Name: "force", Description: "force upload"},
		{Name: "dry-run", Description: "preview changes"},
		{Name: "max-bytes", Value: "n", Description: "chunk size"},
		{Name: "json", Description: "emit JSON"},
		{Name: "exclude", Aliases: []string{"x"}, Value: "pattern", Description: "exclude pattern"},
		{Name: "include-untracked", Description: "include untracked files"},
		{Name: "parallel", Value: "n", Description: "parallel uploads"},
		{Name: "pre-process", Value: "command", Description: "pre-process command"},
	}
	configureTypedCommandSpecWithUsage(spec,
		[]commandForm{{
			Parts: []operandSpec{remainingOperand("positionals")},
			Constraints: []constraint{
				constraintFunc(validateSourceSyncCommand),
			},
		}},
		decodeSourceSync,
		func(path string) {
			fmt.Fprintf(os.Stderr, "usage: nlm %s <notebook-id> [paths...]\n", path)
		},
	)
}

func configureSourcePackSpec(spec *commandSpec) {
	spec.Flags = []flagSpec{
		{Name: "name", Aliases: []string{"n"}, Value: "name", Description: "source name"},
		{Name: "max-bytes", Value: "n", Description: "chunk size"},
		{Name: "chunk", Value: "n", Description: "chunk number"},
		{Name: "exclude", Aliases: []string{"x"}, Value: "pattern", Description: "exclude pattern"},
		{Name: "pre-process", Value: "command", Description: "pre-process command"},
	}
	configureTypedCommandSpecWithUsage(spec,
		[]commandForm{{
			Parts: []operandSpec{remainingOperand("paths")},
			Constraints: []constraint{
				constraintFunc(validateSourcePackCommand),
			},
		}},
		decodeSourcePack,
		func(path string) {
			fmt.Fprintf(os.Stderr, "usage: nlm %s [paths...]\n", path)
		},
	)
}

func validateSourceAddCommand(parsed parsedCommand) error {
	_, err := decodeSourceAddArgs(parsed)
	return err
}

func decodeSourceAdd(parsed parsedCommand) (commandCall, error) {
	args, err := decodeSourceAddArgs(parsed)
	if err != nil {
		return nil, err
	}
	return func(_ context.Context, client *api.Client) error {
		inputs, err := addSourceInputs(args.Inputs)
		if err != nil {
			return err
		}
		return addSources(client, args.NotebookID, inputs, args.Options)
	}, nil
}

func decodeSourceAddArgs(parsed parsedCommand) (sourceAddArgs, error) {
	positionals := parsed.Args["positionals"]
	if len(positionals) < 2 {
		return sourceAddArgs{}, fmt.Errorf("missing notebook id or source")
	}
	notebookID := positionals[0]
	inputs := append([]string(nil), positionals[1:]...)
	chunk, err := parsedIntFlag(parsed, "chunk", 0)
	if err != nil {
		return sourceAddArgs{}, err
	}
	opts := sourceAddOptions{
		Name:            parsedStringFlag(parsed, "name", parsed.globals.sourceName),
		MIMEType:        parsedStringFlag(parsed, "mime", parsed.globals.mimeType),
		ReplaceSourceID: parsedStringFlag(parsed, "replace", parsed.globals.replaceSourceID),
		PreProcess:      parsedStringFlag(parsed, "pre-process", ""),
		Chunk:           chunk,
	}
	if opts.Chunk < 0 {
		return sourceAddArgs{}, fmt.Errorf("--chunk must be >= 0")
	}
	if opts.Chunk > api.MaxTextSourceBytes {
		return sourceAddArgs{}, fmt.Errorf("--chunk %d exceeds per-request limit %d", opts.Chunk, api.MaxTextSourceBytes)
	}
	if opts.ReplaceSourceID != "" && len(inputs) != 1 {
		return sourceAddArgs{}, fmt.Errorf("--replace requires exactly one source")
	}
	return sourceAddArgs{NotebookID: notebookID, Inputs: inputs, Options: opts}, nil
}

func validateSourceSyncCommand(parsed parsedCommand) error {
	_, err := decodeSourceSyncArgs(parsed)
	return err
}

func decodeSourceSync(parsed parsedCommand) (commandCall, error) {
	args, err := decodeSourceSyncArgs(parsed)
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context, client *api.Client) error {
		syncOpts := nlmsync.Options{
			MaxBytes:         args.Options.MaxBytes,
			Name:             args.Options.Name,
			Force:            args.Options.Force,
			DryRun:           args.Options.DryRun,
			JSON:             args.Options.JSON,
			Exclude:          args.Options.Exclude,
			IncludeUntracked: args.Options.IncludeUntracked,
			Parallel:         args.Options.Parallel,
			PreProcess:       args.Options.PreProcess,
		}
		adapter := &syncClientAdapter{client: client}
		return nlmsync.Run(ctx, adapter, args.NotebookID, args.Paths, syncOpts, os.Stdout)
	}, nil
}

func decodeSourceSyncArgs(parsed parsedCommand) (sourceSyncArgs, error) {
	positionals := parsed.Args["positionals"]
	if len(positionals) == 0 {
		return sourceSyncArgs{}, fmt.Errorf("missing notebook id")
	}
	notebookID := positionals[0]
	rawPaths := positionals[1:]
	force, err := parsedBoolFlag(parsed, "force", parsed.globals.force)
	if err != nil {
		return sourceSyncArgs{}, err
	}
	dryRun, err := parsedBoolFlag(parsed, "dry-run", parsed.globals.dryRun)
	if err != nil {
		return sourceSyncArgs{}, err
	}
	maxBytes, err := parsedIntFlag(parsed, "max-bytes", parsed.globals.maxBytes)
	if err != nil {
		return sourceSyncArgs{}, err
	}
	jsonOutput, err := parsedBoolFlag(parsed, "json", parsed.globals.jsonOutput)
	if err != nil {
		return sourceSyncArgs{}, err
	}
	includeUntracked, err := parsedBoolFlag(parsed, "include-untracked", false)
	if err != nil {
		return sourceSyncArgs{}, err
	}
	parallel, err := parsedIntFlag(parsed, "parallel", 0)
	if err != nil {
		return sourceSyncArgs{}, err
	}
	if maxBytes < 0 {
		return sourceSyncArgs{}, fmt.Errorf("--max-bytes must be >= 0")
	}
	paths := append([]string(nil), rawPaths...)
	switch {
	case len(paths) == 0:
		paths = []string{"."}
	case paths[0] == "-":
		paths = nil
	}
	return sourceSyncArgs{
		NotebookID: notebookID,
		Paths:      paths,
		Options: syncOptions{
			Name:             parsedStringFlag(parsed, "name", parsed.globals.sourceName),
			Force:            force,
			DryRun:           dryRun,
			MaxBytes:         maxBytes,
			JSON:             jsonOutput,
			Exclude:          append([]string(nil), parsed.Flags["exclude"]...),
			IncludeUntracked: includeUntracked,
			Parallel:         parallel,
			PreProcess:       parsedStringFlag(parsed, "pre-process", ""),
		},
	}, nil
}

func validateSourcePackCommand(parsed parsedCommand) error {
	_, err := decodeSourcePackArgs(parsed)
	return err
}

func decodeSourcePack(parsed parsedCommand) (commandCall, error) {
	args, err := decodeSourcePackArgs(parsed)
	if err != nil {
		return nil, err
	}
	return func(context.Context, *api.Client) error {
		return runSyncPack(args.Paths, args.Options)
	}, nil
}

func decodeSourcePackArgs(parsed parsedCommand) (sourcePackArgs, error) {
	maxBytes, err := parsedIntFlag(parsed, "max-bytes", parsed.globals.maxBytes)
	if err != nil {
		return sourcePackArgs{}, err
	}
	chunk, err := parsedIntFlag(parsed, "chunk", parsed.globals.packChunk)
	if err != nil {
		return sourcePackArgs{}, err
	}
	if maxBytes < 0 {
		return sourcePackArgs{}, fmt.Errorf("--max-bytes must be >= 0")
	}
	if chunk < 0 {
		return sourcePackArgs{}, fmt.Errorf("--chunk must be >= 0")
	}
	return sourcePackArgs{
		Paths: append([]string(nil), parsed.Args["paths"]...),
		Options: syncPackOptions{
			Name:       parsedStringFlag(parsed, "name", parsed.globals.sourceName),
			MaxBytes:   maxBytes,
			Chunk:      chunk,
			Exclude:    append([]string(nil), parsed.Flags["exclude"]...),
			PreProcess: parsedStringFlag(parsed, "pre-process", ""),
		},
	}, nil
}

func validateSourceReadCommand(parsed parsedCommand) error {
	_, err := decodeSourceReadArgs(parsed)
	return err
}

func decodeSourceRead(parsed parsedCommand) (commandCall, error) {
	args, err := decodeSourceReadArgs(parsed)
	if err != nil {
		return nil, err
	}
	return func(_ context.Context, client *api.Client) error {
		warnDeprecatedSourceReadFormat(os.Stderr, args.Warnings)
		return readSource(client, args.SourceID, args.NotebookID, args.Options)
	}, nil
}

func decodeSourceReadArgs(parsed parsedCommand) (sourceReadArgs, error) {
	sourceID, err := parsedArgument(parsed, "source")
	if err != nil {
		return sourceReadArgs{}, err
	}
	notebookID, _, err := parsedOptionalArgument(parsed, "notebook")
	if err != nil {
		return sourceReadArgs{}, err
	}
	opts := parsed.globals
	opts.sourceReadFormat = parsedStringFlag(parsed, "format", opts.sourceReadFormat)
	if err := normalizeSourceReadFormat(&opts); err != nil {
		return sourceReadArgs{}, err
	}
	return sourceReadArgs{
		SourceID:   sourceID,
		NotebookID: notebookID,
		Options:    opts,
		Warnings:   parsed.globals,
	}, nil
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
