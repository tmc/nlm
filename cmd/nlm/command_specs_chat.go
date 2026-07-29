package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/tmc/nlm/internal/notebooklm/api"
)

type generateChatArgs struct {
	NotebookID string
	Prompt     string
	Options    generateChatOptions
}

type chatArgs struct {
	NotebookID string
	Rest       []string
	Options    chatOptions
}

type chatShowArgs struct {
	NotebookID     string
	ConversationID string
	Options        chatRenderOptions
}

type createReportArgs struct {
	NotebookID string
	ReportType string
	Extra      []string
	Options    createReportOptions
}

type generateReportArgs struct {
	NotebookID string
	Options    reportOptions
}

func configureChatCommandSpecs(specs map[commandID]*commandSpec) {
	generateSpec := specs["generate-chat"]
	generateSpec.Flags = append(
		[]flagSpec{
			{Name: "conversation", Aliases: []string{"c"}, Value: "id", Description: "conversation ID"},
			{Name: "web", Description: "use server-side conversation"},
			{Name: "prompt-file", Aliases: []string{"f"}, Value: "path", Description: "prompt file"},
		},
		append(chatRenderFlagSpecs(), selectorFlagSpecs()...)...,
	)
	configureTypedCommandSpecWithUsage(
		generateSpec,
		chatCommandForm("<notebook-id> [prompt...]", validateGenerateChatCommand),
		decodeGenerateChat,
		printGenerateChatErrorUsage,
	)

	chatSpec := specs["chat"]
	chatSpec.Flags = append(
		[]flagSpec{
			{Name: "prompt-file", Aliases: []string{"f"}, Value: "path", Description: "prompt file"},
			{Name: "history", Description: "show conversation history"},
		},
		append(chatRenderFlagSpecs(), selectorFlagSpecs()...)...,
	)
	configureTypedCommandSpecWithUsage(
		chatSpec,
		chatCommandForm("<notebook-id> [conversation-id | prompt...]", validateChatCommand),
		decodeChat,
		printChatErrorUsage,
	)

	showSpec := specs["chat-show"]
	showSpec.Flags = append(chatShowRenderFlagSpecs(),
		flagSpec{Name: "format", Value: "fmt", Description: "output format"},
		flagSpec{Name: "out", Value: "file", Description: "output file"},
		flagSpec{Name: "open", Description: "open HTML"},
		flagSpec{Name: "include-follow-ups", Description: "include follow-up prompts"},
		flagSpec{Name: "backfill", Description: "backfill saved conversation"},
	)
	configureTypedCommandSpecWithUsage(
		showSpec,
		chatCommandForm("<notebook-id> [conversation-id]", validateChatShowCommand),
		decodeChatShow,
		printChatShowErrorUsage,
	)

	createReportSpec := specs["create-report"]
	createReportSpec.Flags = selectorFlagSpecs()
	configureTypedCommandSpecWithUsage(
		createReportSpec,
		chatCommandForm("<notebook-id> <report-type> [description...]", validateCreateReportCommand),
		decodeCreateReport,
		printCreateReportErrorUsage,
	)

	reportSpec := specs["generate-report"]
	reportSpec.Flags = append(
		[]flagSpec{
			{Name: "prompt", Value: "template", Description: "section prompt"},
			{Name: "instructions", Value: "text", Description: "notebook instructions"},
			{Name: "sections", Value: "n", Description: "section count"},
		},
		append(chatRenderFlagSpecs(), selectorFlagSpecs()...)...,
	)
	configureTypedCommandSpecWithUsage(
		reportSpec,
		chatCommandForm("<notebook-id>", validateGenerateReportCommand),
		decodeGenerateReport,
		printGenerateReportErrorUsage,
	)
}

func chatCommandForm(usage string, validate func(parsedCommand) error) []commandForm {
	return []commandForm{{
		Parts:       []operandSpec{withUsage(remainingOperand("positionals"), usage)},
		Constraints: []constraint{constraintFunc(validate)},
	}}
}

func chatRenderFlagSpecs() []flagSpec {
	return []flagSpec{
		{Name: "thinking", Aliases: []string{"reasoning"}, Description: "show thinking"},
		{Name: "thinking-jsonl", Description: "emit thinking JSON lines", Visibility: flagDeprecated},
		{Name: "verbose", Aliases: []string{"v"}, Description: "show thinking traces"},
		{Name: "citations", Value: "mode", Description: "citation rendering"},
		{Name: "resolve-citations", Description: "resolve citations"},
		{
			Name:          "citation-excerpts",
			Aliases:       []string{"citation-excerpt"},
			Value:         "n",
			OptionalValue: true,
			Description:   "show citation excerpts",
		},
		{
			Name:          "citation-confidence",
			Value:         "on|off",
			OptionalValue: true,
			Description:   "show citation confidence",
		},
		{
			Name:          "citation-spans",
			Value:         "on|off",
			OptionalValue: true,
			Description:   "show citation spans",
		},
	}
}

func chatShowRenderFlagSpecs() []flagSpec {
	flags := chatRenderFlagSpecs()
	out := make([]flagSpec, 0, len(flags)-2)
	for _, flag := range flags {
		if flag.Name != "thinking-jsonl" && flag.Name != "verbose" {
			out = append(out, flag)
		}
	}
	return out
}

func printGenerateChatErrorUsage(path string) {
	printCommandUsageForPath(path)
}

func printChatErrorUsage(path string) {
	printCommandUsageForPath(path)
}

func printChatShowErrorUsage(path string) {
	printCommandUsageForPath(path)
}

func printCreateReportErrorUsage(path string) {
	printCommandUsageForPath(path)
}

func printGenerateReportErrorUsage(path string) {
	printCommandUsageForPath(path)
}

func validateGenerateChatCommand(parsed parsedCommand) error {
	_, err := decodeGenerateChatArgs(parsed)
	return err
}

func decodeGenerateChat(parsed parsedCommand) (commandCall, error) {
	args, err := decodeGenerateChatArgs(parsed)
	if err != nil {
		return nil, err
	}
	return func(_ context.Context, client *api.Client) error {
		prompt := args.Prompt
		if args.Options.PromptFile != "" {
			prompt, err = readPromptFile(args.Options.PromptFile)
			if err != nil {
				return fmt.Errorf("read prompt: %w", err)
			}
		}
		return generateFreeFormChat(client, args.NotebookID, prompt, args.Options)
	}, nil
}

func decodeGenerateChatArgs(parsed parsedCommand) (generateChatArgs, error) {
	positionals := parsed.Args["positionals"]
	options, err := decodeGenerateChatOptions(parsed)
	if err != nil {
		return generateChatArgs{}, err
	}
	if len(positionals) == 0 {
		return generateChatArgs{}, fmt.Errorf("missing notebook id")
	}
	if options.PromptFile == "" && len(positionals) < 2 {
		return generateChatArgs{}, fmt.Errorf("missing prompt")
	}
	return generateChatArgs{
		NotebookID: positionals[0],
		Prompt:     strings.Join(positionals[1:], " "),
		Options:    options,
	}, nil
}

func decodeGenerateChatOptions(parsed parsedCommand) (generateChatOptions, error) {
	useWebChat, err := parsedBoolFlag(parsed, "web", parsed.globals.useWebChat)
	if err != nil {
		return generateChatOptions{}, err
	}
	render, err := decodeChatRenderOptions(parsed)
	if err != nil {
		return generateChatOptions{}, err
	}
	return generateChatOptions{
		ConversationID: parsedStringFlag(parsed, "conversation", parsed.globals.conversationID),
		UseWebChat:     useWebChat,
		PromptFile:     parsedStringFlag(parsed, "prompt-file", parsed.globals.promptFile),
		Selectors:      decodeSelectorOptions(parsed),
		Render:         render,
	}, nil
}

func validateChatCommand(parsed parsedCommand) error {
	_, err := decodeChatArgs(parsed)
	return err
}

func decodeChat(parsed parsedCommand) (commandCall, error) {
	args, err := decodeChatArgs(parsed)
	if err != nil {
		return nil, err
	}
	return func(_ context.Context, client *api.Client) error {
		if args.Options.PromptFile != "" {
			prompt, err := readPromptFile(args.Options.PromptFile)
			if err != nil {
				return fmt.Errorf("read prompt: %w", err)
			}
			if len(args.Rest) >= 1 && isConversationID(args.Rest[0]) {
				return oneShotChatInConv(client, args.NotebookID, args.Rest[0], prompt, args.Options)
			}
			return oneShotChat(client, args.NotebookID, prompt, args.Options)
		}
		if len(args.Rest) >= 1 {
			rest := strings.Join(args.Rest, " ")
			if isConversationID(rest) {
				return interactiveChatWithConv(client, args.NotebookID, rest, args.Options)
			}
			return oneShotChat(client, args.NotebookID, rest, args.Options)
		}
		return interactiveChat(client, args.NotebookID, args.Options)
	}, nil
}

func decodeChatArgs(parsed parsedCommand) (chatArgs, error) {
	positionals := parsed.Args["positionals"]
	if len(positionals) == 0 {
		return chatArgs{}, fmt.Errorf("missing notebook id")
	}
	showHistory, err := parsedBoolFlag(parsed, "history", parsed.globals.showChatHistory)
	if err != nil {
		return chatArgs{}, err
	}
	render, err := decodeChatRenderOptions(parsed)
	if err != nil {
		return chatArgs{}, err
	}
	return chatArgs{
		NotebookID: positionals[0],
		Rest:       append([]string(nil), positionals[1:]...),
		Options: chatOptions{
			PromptFile:  parsedStringFlag(parsed, "prompt-file", parsed.globals.promptFile),
			ShowHistory: showHistory,
			Selectors:   decodeSelectorOptions(parsed),
			Render:      render,
		},
	}, nil
}

func validateChatShowCommand(parsed parsedCommand) error {
	_, err := decodeChatShowArgs(parsed)
	return err
}

func decodeChatShow(parsed parsedCommand) (commandCall, error) {
	args, err := decodeChatShowArgs(parsed)
	if err != nil {
		return nil, err
	}
	return func(_ context.Context, _ *api.Client) error {
		if args.ConversationID == "" {
			return chatShowNotebook(args.NotebookID, args.Options)
		}
		return chatShow(args.NotebookID, args.ConversationID, args.Options)
	}, nil
}

func decodeChatShowArgs(parsed parsedCommand) (chatShowArgs, error) {
	positionals := parsed.Args["positionals"]
	if len(positionals) < 1 || len(positionals) > 2 {
		return chatShowArgs{}, fmt.Errorf("want notebook id and optional conversation id")
	}
	options, err := decodeChatRenderOptions(parsed)
	if err != nil {
		return chatShowArgs{}, err
	}
	options.Format = parsedStringFlag(parsed, "format", options.Format)
	options.OutFile = parsedStringFlag(parsed, "out", options.OutFile)
	options.Open, err = parsedBoolFlag(parsed, "open", options.Open)
	if err != nil {
		return chatShowArgs{}, err
	}
	options.IncludeFollowUps, err = parsedBoolFlag(parsed, "include-follow-ups", options.IncludeFollowUps)
	if err != nil {
		return chatShowArgs{}, err
	}
	options.Backfill, err = parsedBoolFlag(parsed, "backfill", options.Backfill)
	if err != nil {
		return chatShowArgs{}, err
	}
	if len(positionals) == 1 && options.Format == "" {
		options.Format = "html"
	}
	if err := validateChatFormat(&options); err != nil {
		return chatShowArgs{}, err
	}
	if len(positionals) == 1 && options.Format != "html" {
		return chatShowArgs{}, fmt.Errorf("whole-notebook render requires --format=html")
	}
	if len(positionals) == 1 && options.Backfill {
		return chatShowArgs{}, fmt.Errorf("--backfill requires a conversation id")
	}
	args := chatShowArgs{
		NotebookID: positionals[0],
		Options:    options,
	}
	if len(positionals) == 2 {
		args.ConversationID = positionals[1]
	}
	return args, nil
}

func validateCreateReportCommand(parsed parsedCommand) error {
	_, err := decodeCreateReportArgs(parsed)
	return err
}

func decodeCreateReport(parsed parsedCommand) (commandCall, error) {
	args, err := decodeCreateReportArgs(parsed)
	if err != nil {
		return nil, err
	}
	return func(_ context.Context, client *api.Client) error {
		return createReport(client, args.NotebookID, args.ReportType, args.Extra, args.Options)
	}, nil
}

func decodeCreateReportArgs(parsed parsedCommand) (createReportArgs, error) {
	positionals := parsed.Args["positionals"]
	if len(positionals) < 2 {
		return createReportArgs{}, fmt.Errorf("want notebook id and report type")
	}
	return createReportArgs{
		NotebookID: positionals[0],
		ReportType: positionals[1],
		Extra:      append([]string(nil), positionals[2:]...),
		Options:    createReportOptions{Selectors: decodeSelectorOptions(parsed)},
	}, nil
}

func validateGenerateReportCommand(parsed parsedCommand) error {
	_, err := decodeGenerateReportArgs(parsed)
	return err
}

func decodeGenerateReport(parsed parsedCommand) (commandCall, error) {
	args, err := decodeGenerateReportArgs(parsed)
	if err != nil {
		return nil, err
	}
	return func(_ context.Context, client *api.Client) error {
		return generateReport(client, args.NotebookID, args.Options)
	}, nil
}

func decodeGenerateReportArgs(parsed parsedCommand) (generateReportArgs, error) {
	positionals := parsed.Args["positionals"]
	if len(positionals) != 1 {
		return generateReportArgs{}, fmt.Errorf("want notebook id")
	}
	sections, err := parsedIntFlag(parsed, "sections", parsed.globals.reportSections)
	if err != nil {
		return generateReportArgs{}, err
	}
	if sections < 0 {
		return generateReportArgs{}, fmt.Errorf("--sections must be >= 0")
	}
	render, err := decodeChatRenderOptions(parsed)
	if err != nil {
		return generateReportArgs{}, err
	}
	return generateReportArgs{
		NotebookID: positionals[0],
		Options: reportOptions{
			Prompt:       parsedStringFlag(parsed, "prompt", parsed.globals.reportPrompt),
			Instructions: parsedStringFlag(parsed, "instructions", parsed.globals.reportInstructions),
			Sections:     sections,
			Selectors:    decodeSelectorOptions(parsed),
			Render:       render,
		},
	}, nil
}

func decodeChatRenderOptions(parsed parsedCommand) (chatRenderOptions, error) {
	options := chatRenderOptionsFromGlobals(parsed.globals)
	var err error
	options.ShowThinking, err = parsedBoolFlag(parsed, "thinking", options.ShowThinking)
	if err != nil {
		return chatRenderOptions{}, err
	}
	options.ThinkingJSONL, err = parsedBoolFlag(parsed, "thinking-jsonl", options.ThinkingJSONL)
	if err != nil {
		return chatRenderOptions{}, err
	}
	options.Verbose, err = parsedBoolFlag(parsed, "verbose", options.Verbose)
	if err != nil {
		return chatRenderOptions{}, err
	}
	options.CitationMode = parsedStringFlag(parsed, "citations", options.CitationMode)
	options.ResolveCitations, err = parsedBoolFlag(parsed, "resolve-citations", options.ResolveCitations)
	if err != nil {
		return chatRenderOptions{}, err
	}
	options.ExcerptBudget, err = parsedExcerptBudget(parsed, options.ExcerptBudget)
	if err != nil {
		return chatRenderOptions{}, err
	}
	options.HideConfidence, err = parsedOffToggle(parsed, "citation-confidence", options.HideConfidence)
	if err != nil {
		return chatRenderOptions{}, err
	}
	options.HideSpans, err = parsedOffToggle(parsed, "citation-spans", options.HideSpans)
	if err != nil {
		return chatRenderOptions{}, err
	}
	return options, nil
}

func parsedExcerptBudget(parsed parsedCommand, defaultValue int) (int, error) {
	values := parsed.Flags["citation-excerpts"]
	if len(values) == 0 {
		return defaultValue, nil
	}
	var value excerptBudgetFlag
	for _, raw := range values {
		if err := value.Set(raw); err != nil {
			return 0, fmt.Errorf("invalid value %q for flag -citation-excerpts: %w", raw, err)
		}
	}
	return value.Budget(), nil
}

func parsedOffToggle(parsed parsedCommand, name string, defaultValue bool) (bool, error) {
	values := parsed.Flags[name]
	if len(values) == 0 {
		return defaultValue, nil
	}
	hidden := defaultValue
	for _, raw := range values {
		var value offToggleFlag
		if err := value.Set(raw); err != nil {
			return false, fmt.Errorf("invalid value %q for flag -%s: %w", raw, name, err)
		}
		hidden = value.Hidden()
	}
	return hidden, nil
}
