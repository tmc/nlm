package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/tmc/nlm/internal/notebooklm/api"
)

type chatRenderOptions struct {
	ShowThinking     bool
	ThinkingJSONL    bool
	Verbose          bool
	CitationMode     string
	ResolveCitations bool
	ExcerptBudget    int  // >0 shows the cited source span under each citation, clipped to this many chars
	HideConfidence   bool // --citation-confidence=off: drop the (p=) column from the citation list
	HideSpans        bool // --citation-spans=off: drop the trailing [chars N-M] from citation rows
	IncludeFollowUps bool // --include-follow-ups: retain generated trailing prompts in HTML
	Backfill         bool // --backfill: persist missing citations and rich trees from server history

	// Whole-document output format for chat-show: "" (text, default),
	// "markdown", or "html". OutFile and Open apply to html.
	Format  string
	OutFile string // --out FILE: write html here; "-" writes to stdout
	Open    bool   // --open: open the written html file in the browser
}

type generateChatOptions struct {
	ConversationID string
	UseWebChat     bool
	PromptFile     string
	Selectors      selectorOptions
	Render         chatRenderOptions
}

type chatOptions struct {
	PromptFile  string
	ShowHistory bool
	Selectors   selectorOptions
	Render      chatRenderOptions
}

type reportOptions struct {
	Prompt       string
	Instructions string
	Sections     int
	Selectors    selectorOptions
	Render       chatRenderOptions
}

type createReportOptions struct {
	Selectors selectorOptions
}

func currentChatRenderOptions() chatRenderOptions {
	return chatRenderOptionsFromGlobals(packageGlobalOptions())
}

func chatRenderOptionsFromGlobals(globals globalOptions) chatRenderOptions {
	return chatRenderOptions{
		ShowThinking:     globals.showThinking,
		ThinkingJSONL:    globals.thinkingJSONL,
		Verbose:          globals.verbose,
		CitationMode:     globals.citationMode,
		ResolveCitations: globals.resolveCitationsFlag,
		ExcerptBudget:    globals.citationExcerpt.Budget(),
		HideConfidence:   globals.hideCitationConf.Hidden(),
		HideSpans:        globals.hideCitationSpans.Hidden(),
	}
}

func appendSelectorFlags(flags *flag.FlagSet, opts *selectorOptions) {
	flags.StringVar(&opts.SourceIDs, "source-ids", opts.SourceIDs, "")
	flags.StringVar(&opts.SourceMatch, "source-match", opts.SourceMatch, "")
	flags.StringVar(&opts.SourceExclude, "source-exclude", opts.SourceExclude, "")
	flags.StringVar(&opts.LabelIDs, "label-ids", opts.LabelIDs, "")
	flags.StringVar(&opts.LabelMatch, "label-match", opts.LabelMatch, "")
	flags.StringVar(&opts.LabelExclude, "label-exclude", opts.LabelExclude, "")
}

func appendRenderFlags(flags *flag.FlagSet, opts *chatRenderOptions) {
	flags.BoolVar(&opts.ShowThinking, "thinking", opts.ShowThinking, "")
	flags.BoolVar(&opts.ShowThinking, "reasoning", opts.ShowThinking, "")
	flags.BoolVar(&opts.ThinkingJSONL, "thinking-jsonl", opts.ThinkingJSONL, "")
	flags.BoolVar(&opts.Verbose, "verbose", opts.Verbose, "")
	flags.BoolVar(&opts.Verbose, "v", opts.Verbose, "")
	flags.StringVar(&opts.CitationMode, "citations", opts.CitationMode, "")
	flags.BoolVar(&opts.ResolveCitations, "resolve-citations", opts.ResolveCitations, "")
	sink := &excerptBudgetSink{dst: &opts.ExcerptBudget}
	flags.Var(sink, "citation-excerpts", "")
	flags.Var(sink, "citation-excerpt", "") // singular alias
	flags.Var(&offToggleSink{hidden: &opts.HideConfidence}, "citation-confidence", "")
	flags.Var(&offToggleSink{hidden: &opts.HideSpans}, "citation-spans", "")
}

func printGenerateChatUsage(cmdName string) {
	fmt.Fprintf(os.Stderr, "Usage: nlm %s [flags] <notebook-id> [prompt]\n\n", cmdName)
	fmt.Fprintln(os.Stderr, "Flags:")
	fmt.Fprintln(os.Stderr, "  --conversation, -c <id>  Continue an existing conversation by ID")
	fmt.Fprintln(os.Stderr, "  --web                    Use the most recent server-side conversation")
	fmt.Fprintln(os.Stderr, "  --prompt-file, -f <path> Read the prompt from a file ('-' reads stdin)")
	fmt.Fprintln(os.Stderr, "  --thinking, --reasoning  Show thinking headers while streaming")
	fmt.Fprintln(os.Stderr, "  --verbose, -v            Show full thinking traces while streaming")
	fmt.Fprintln(os.Stderr, "  --citations <mode>       Citation rendering: off|list|json (default list; block/stream/tail are deprecated aliases of list)")
	fmt.Fprintln(os.Stderr, "  --citation-confidence=off  Hide the (p=…) confidence column in the citation list")
	fmt.Fprintln(os.Stderr, "  --citation-spans=off       Hide the trailing [chars N-M] span column in the citation list")
	fmt.Fprintln(os.Stderr, "  --resolve-citations      Resolve citations to file:line for txtar-archive sources")
	fmt.Fprintln(os.Stderr, "  --citation-excerpts[=N]  Show the cited source text under each citation (N chars, default 160)")
	fmt.Fprintln(os.Stderr, "  --source-ids <ids>       Focus on these source IDs ('a,b,c' or '-' for stdin)")
	fmt.Fprintln(os.Stderr, "  --source-match <regex>   Focus on sources whose title or UUID matches the regex")
	fmt.Fprintln(os.Stderr, "  --source-exclude <regex> Exclude sources whose title or UUID matches the regex")
	fmt.Fprintln(os.Stderr, "  --label-ids <ids>        Include sources tagged with any of these label IDs")
	fmt.Fprintln(os.Stderr, "  --label-match <regex>    Include sources tagged with any label whose name matches the regex")
	fmt.Fprintln(os.Stderr, "  --label-exclude <regex>  Exclude sources tagged with any label whose name matches the regex")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Examples:")
	fmt.Fprintf(os.Stderr, "  nlm %s <notebook-id> \"Summarize the architecture\"\n", cmdName)
	fmt.Fprintf(os.Stderr, "  nlm %s --prompt-file prompt.txt <notebook-id>\n", cmdName)
	fmt.Fprintf(os.Stderr, "  nlm %s --conversation <id> <notebook-id> \"Follow up on section 2\"\n", cmdName)
}

func validateGenerateChatArgs(cmdName string, args []string) error {
	return validateGenerateChatArgsWithOptions(cmdName, args, packageGlobalOptions())
}

func validateGenerateChatArgsWithOptions(cmdName string, args []string, globals globalOptions) error {
	opts, positional, err := parseGenerateChatArgsWithOptions(args, globals)
	if err == nil && len(positional) >= 1 && (opts.PromptFile != "" || len(positional) >= 2) {
		return nil
	}
	fmt.Fprintf(os.Stderr, "usage: nlm %s <notebook-id> [prompt]\n", cmdName)
	return errBadArgs
}

func parseGenerateChatArgs(args []string) (generateChatOptions, []string, error) {
	return parseGenerateChatArgsWithOptions(args, packageGlobalOptions())
}

func parseGenerateChatArgsWithOptions(args []string, globals globalOptions) (generateChatOptions, []string, error) {
	opts := generateChatOptions{
		ConversationID: globals.conversationID,
		UseWebChat:     globals.useWebChat,
		PromptFile:     globals.promptFile,
		Selectors:      selectorOptionsFromGlobals(globals),
		Render:         chatRenderOptionsFromGlobals(globals),
	}
	flags := flag.NewFlagSet("generate-chat", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&opts.ConversationID, "conversation", opts.ConversationID, "")
	flags.StringVar(&opts.ConversationID, "c", opts.ConversationID, "")
	flags.BoolVar(&opts.UseWebChat, "web", opts.UseWebChat, "")
	flags.StringVar(&opts.PromptFile, "prompt-file", opts.PromptFile, "")
	flags.StringVar(&opts.PromptFile, "f", opts.PromptFile, "")
	appendRenderFlags(flags, &opts.Render)
	appendSelectorFlags(flags, &opts.Selectors)

	flagArgs, positional, err := splitCommandFlags(args, map[string]bool{
		"conversation":        true,
		"c":                   true,
		"web":                 true,
		"prompt-file":         true,
		"f":                   true,
		"thinking":            true,
		"reasoning":           true,
		"thinking-jsonl":      true,
		"verbose":             true,
		"v":                   true,
		"citations":           true,
		"resolve-citations":   true,
		"citation-excerpts":   true,
		"citation-excerpt":    true,
		"citation-confidence": true,
		"citation-spans":      true,
		"source-ids":          true,
		"source-match":        true,
		"source-exclude":      true,
		"label-ids":           true,
		"label-match":         true,
		"label-exclude":       true,
	}, map[string]bool{
		"web":                 true,
		"thinking":            true,
		"reasoning":           true,
		"thinking-jsonl":      true,
		"verbose":             true,
		"v":                   true,
		"resolve-citations":   true,
		"citation-excerpts":   true,
		"citation-excerpt":    true,
		"citation-confidence": true,
		"citation-spans":      true,
	})
	if err != nil {
		return opts, nil, err
	}
	if err := flags.Parse(flagArgs); err != nil {
		return opts, nil, err
	}
	if len(positional) == 0 {
		return opts, nil, fmt.Errorf("missing notebook id")
	}
	if opts.PromptFile == "" && len(positional) < 2 {
		return opts, nil, fmt.Errorf("missing prompt")
	}
	return opts, positional, nil
}

func runGenerateChat(c *api.Client, args []string) error {
	return runGenerateChatWithOptions(c, args, packageGlobalOptions())
}

func runGenerateChatWithOptions(c *api.Client, args []string, globals globalOptions) error {
	opts, positional, err := parseGenerateChatArgsWithOptions(args, globals)
	if err != nil {
		return err
	}
	prompt := strings.Join(positional[1:], " ")
	if opts.PromptFile != "" {
		prompt, err = readPromptFile(opts.PromptFile)
		if err != nil {
			return fmt.Errorf("read prompt: %w", err)
		}
	}
	return generateFreeFormChat(c, positional[0], prompt, opts)
}

func printChatUsage(cmdName string) {
	fmt.Fprintf(os.Stderr, "Usage: nlm %s [flags] <notebook-id> [conversation-id | prompt]\n\n", cmdName)
	fmt.Fprintln(os.Stderr, "Flags:")
	fmt.Fprintln(os.Stderr, "  --prompt-file, -f <path> Read the prompt from a file ('-' reads stdin)")
	fmt.Fprintln(os.Stderr, "  --history                Show previous chat conversation on start")
	fmt.Fprintln(os.Stderr, "  --thinking, --reasoning  Show thinking headers while streaming")
	fmt.Fprintln(os.Stderr, "  --verbose, -v            Show full thinking traces while streaming")
	fmt.Fprintln(os.Stderr, "  --citations <mode>       Citation rendering: off|list|json (default list; block/stream/tail are deprecated aliases of list)")
	fmt.Fprintln(os.Stderr, "  --citation-confidence=off  Hide the (p=…) confidence column in the citation list")
	fmt.Fprintln(os.Stderr, "  --citation-spans=off       Hide the trailing [chars N-M] span column in the citation list")
	fmt.Fprintln(os.Stderr, "  --resolve-citations      Resolve citations to file:line for txtar-archive sources")
	fmt.Fprintln(os.Stderr, "  --citation-excerpts[=N]  Show the cited source text under each citation (N chars, default 160)")
	fmt.Fprintln(os.Stderr, "  --source-ids <ids>       Focus on these source IDs ('a,b,c' or '-' for stdin)")
	fmt.Fprintln(os.Stderr, "  --source-match <regex>   Focus on sources whose title or UUID matches the regex")
	fmt.Fprintln(os.Stderr, "  --source-exclude <regex> Exclude sources whose title or UUID matches the regex")
	fmt.Fprintln(os.Stderr, "  --label-ids <ids>        Include sources tagged with any of these label IDs")
	fmt.Fprintln(os.Stderr, "  --label-match <regex>    Include sources tagged with any label whose name matches the regex")
	fmt.Fprintln(os.Stderr, "  --label-exclude <regex>  Exclude sources tagged with any label whose name matches the regex")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Examples:")
	fmt.Fprintf(os.Stderr, "  nlm %s <notebook-id>\n", cmdName)
	fmt.Fprintf(os.Stderr, "  nlm %s <notebook-id> \"What changed this week?\"\n", cmdName)
	fmt.Fprintf(os.Stderr, "  nlm %s --prompt-file prompt.txt <notebook-id>\n", cmdName)
}

func validateChatArgs(cmdName string, args []string) error {
	return validateChatArgsWithOptions(cmdName, args, packageGlobalOptions())
}

func validateChatArgsWithOptions(cmdName string, args []string, globals globalOptions) error {
	_, positional, err := parseChatArgsWithOptions(args, globals)
	if err == nil && len(positional) >= 1 {
		return nil
	}
	fmt.Fprintf(os.Stderr, "usage: nlm %s <notebook-id> [conversation-id | prompt]\n", cmdName)
	return errBadArgs
}

func parseChatArgs(args []string) (chatOptions, []string, error) {
	return parseChatArgsWithOptions(args, packageGlobalOptions())
}

func parseChatArgsWithOptions(args []string, globals globalOptions) (chatOptions, []string, error) {
	opts := chatOptions{
		PromptFile:  globals.promptFile,
		ShowHistory: globals.showChatHistory,
		Selectors:   selectorOptionsFromGlobals(globals),
		Render:      chatRenderOptionsFromGlobals(globals),
	}
	flags := flag.NewFlagSet("chat", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&opts.PromptFile, "prompt-file", opts.PromptFile, "")
	flags.StringVar(&opts.PromptFile, "f", opts.PromptFile, "")
	flags.BoolVar(&opts.ShowHistory, "history", opts.ShowHistory, "")
	appendRenderFlags(flags, &opts.Render)
	appendSelectorFlags(flags, &opts.Selectors)

	flagArgs, positional, err := splitCommandFlags(args, map[string]bool{
		"prompt-file":         true,
		"f":                   true,
		"history":             true,
		"thinking":            true,
		"reasoning":           true,
		"thinking-jsonl":      true,
		"verbose":             true,
		"v":                   true,
		"citations":           true,
		"resolve-citations":   true,
		"citation-excerpts":   true,
		"citation-excerpt":    true,
		"citation-confidence": true,
		"citation-spans":      true,
		"source-ids":          true,
		"source-match":        true,
		"source-exclude":      true,
		"label-ids":           true,
		"label-match":         true,
		"label-exclude":       true,
	}, map[string]bool{
		"history":             true,
		"thinking":            true,
		"reasoning":           true,
		"thinking-jsonl":      true,
		"verbose":             true,
		"v":                   true,
		"resolve-citations":   true,
		"citation-excerpts":   true,
		"citation-excerpt":    true,
		"citation-confidence": true,
		"citation-spans":      true,
	})
	if err != nil {
		return opts, nil, err
	}
	if err := flags.Parse(flagArgs); err != nil {
		return opts, nil, err
	}
	if len(positional) == 0 {
		return opts, nil, fmt.Errorf("missing notebook id")
	}
	return opts, positional, nil
}

func runChat(c *api.Client, args []string) error {
	return runChatWithOptions(c, args, packageGlobalOptions())
}

func runChatWithOptions(c *api.Client, args []string, globals globalOptions) error {
	opts, positional, err := parseChatArgsWithOptions(args, globals)
	if err != nil {
		return err
	}
	notebookID := positional[0]
	if opts.PromptFile != "" {
		prompt, err := readPromptFile(opts.PromptFile)
		if err != nil {
			return fmt.Errorf("read prompt: %w", err)
		}
		if len(positional) >= 2 && isConversationID(positional[1]) {
			return oneShotChatInConv(c, notebookID, positional[1], prompt, opts)
		}
		return oneShotChat(c, notebookID, prompt, opts)
	}
	if len(positional) >= 2 {
		rest := strings.Join(positional[1:], " ")
		if isConversationID(rest) {
			return interactiveChatWithConv(c, notebookID, rest, opts)
		}
		return oneShotChat(c, notebookID, rest, opts)
	}
	return interactiveChat(c, notebookID, opts)
}

func printChatShowUsage(cmdName string) {
	fmt.Fprintf(os.Stderr, "Usage: nlm %s [flags] <notebook-id> [conversation-id]\n\n", cmdName)
	fmt.Fprintln(os.Stderr, "Flags:")
	fmt.Fprintln(os.Stderr, "  --thinking, --reasoning  Show persisted thinking traces on stderr")
	fmt.Fprintln(os.Stderr, "  --citations <mode>       Citation rendering: off|list|json (default list; block/stream/tail are deprecated aliases of list)")
	fmt.Fprintln(os.Stderr, "  --citation-confidence=off  Hide the (p=…) confidence column in the citation list")
	fmt.Fprintln(os.Stderr, "  --citation-spans=off       Hide the trailing [chars N-M] span column in the citation list")
	fmt.Fprintln(os.Stderr, "  --resolve-citations      Resolve citations to file:line for txtar-archive sources")
	fmt.Fprintln(os.Stderr, "  --citation-excerpts[=N]  Show the cited source text under each citation (N chars, default 160); rehydrates from the saved conversation")
	fmt.Fprintln(os.Stderr, "  --format <fmt>           Output format: text (default), markdown, or html")
	fmt.Fprintln(os.Stderr, "  --out <file>             Write HTML to file; - writes to stdout (default: render cache)")
	fmt.Fprintln(os.Stderr, "  --open                   Open the written HTML file in a browser (--format=html)")
	fmt.Fprintln(os.Stderr, "  --include-follow-ups     Include generated trailing follow-up prompts in HTML")
	fmt.Fprintln(os.Stderr, "  --backfill               Persist missing citations and rich trees from server history")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "With no conversation ID, renders an HTML notebook switcher.")
}

func validateChatShowArgs(cmdName string, args []string) error {
	return validateChatShowArgsWithOptions(cmdName, args, packageGlobalOptions())
}

func validateChatShowArgsWithOptions(cmdName string, args []string, globals globalOptions) error {
	_, positional, err := parseChatShowArgsWithOptions(args, globals)
	if err == nil && (len(positional) == 1 || len(positional) == 2) {
		return nil
	}
	fmt.Fprintf(os.Stderr, "usage: nlm %s <notebook-id> [conversation-id]\n", cmdName)
	return errBadArgs
}

func parseChatShowArgs(args []string) (chatRenderOptions, []string, error) {
	return parseChatShowArgsWithOptions(args, packageGlobalOptions())
}

func parseChatShowArgsWithOptions(args []string, globals globalOptions) (chatRenderOptions, []string, error) {
	opts := chatRenderOptionsFromGlobals(globals)
	flags := flag.NewFlagSet("chat-show", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.BoolVar(&opts.ShowThinking, "thinking", opts.ShowThinking, "")
	flags.BoolVar(&opts.ShowThinking, "reasoning", opts.ShowThinking, "")
	flags.StringVar(&opts.CitationMode, "citations", opts.CitationMode, "")
	flags.BoolVar(&opts.ResolveCitations, "resolve-citations", opts.ResolveCitations, "")
	excerptSink := &excerptBudgetSink{dst: &opts.ExcerptBudget}
	flags.Var(excerptSink, "citation-excerpts", "")
	flags.Var(excerptSink, "citation-excerpt", "")
	flags.Var(&offToggleSink{hidden: &opts.HideConfidence}, "citation-confidence", "")
	flags.Var(&offToggleSink{hidden: &opts.HideSpans}, "citation-spans", "")
	flags.StringVar(&opts.Format, "format", opts.Format, "")
	flags.StringVar(&opts.OutFile, "out", opts.OutFile, "")
	flags.BoolVar(&opts.Open, "open", opts.Open, "")
	flags.BoolVar(&opts.IncludeFollowUps, "include-follow-ups", opts.IncludeFollowUps, "")
	flags.BoolVar(&opts.Backfill, "backfill", opts.Backfill, "")

	flagArgs, positional, err := splitCommandFlags(args, map[string]bool{
		"thinking":            true,
		"reasoning":           true,
		"citations":           true,
		"resolve-citations":   true,
		"citation-excerpts":   true,
		"citation-excerpt":    true,
		"citation-confidence": true,
		"citation-spans":      true,
		"format":              true,
		"out":                 true,
		"open":                true,
		"include-follow-ups":  true,
		"backfill":            true,
	}, map[string]bool{
		"thinking":            true,
		"reasoning":           true,
		"resolve-citations":   true,
		"citation-excerpts":   true,
		"citation-excerpt":    true,
		"citation-confidence": true,
		"citation-spans":      true,
		"open":                true,
		"include-follow-ups":  true,
		"backfill":            true,
	})
	if err != nil {
		return opts, nil, err
	}
	if err := flags.Parse(flagArgs); err != nil {
		return opts, nil, err
	}
	if len(positional) < 1 || len(positional) > 2 {
		return opts, nil, fmt.Errorf("want notebook id and optional conversation id")
	}
	if len(positional) == 1 && opts.Format == "" {
		opts.Format = "html"
	}
	if err := validateChatFormat(&opts); err != nil {
		return opts, nil, err
	}
	if len(positional) == 1 && opts.Format != "html" {
		return opts, nil, fmt.Errorf("whole-notebook render requires --format=html")
	}
	if len(positional) == 1 && opts.Backfill {
		return opts, nil, fmt.Errorf("--backfill requires a conversation id")
	}
	return opts, positional, nil
}

// validateChatFormat normalizes and checks the --format/--out/--open trio.
// Format defaults to "text"; markdown and html are the alternates. --out and
// --open only apply to html; using them with another format is a usage error
// rather than a silent no-op.
func validateChatFormat(opts *chatRenderOptions) error {
	switch opts.Format {
	case "", "text":
		opts.Format = "text"
	case "markdown", "md":
		opts.Format = "markdown"
	case "html":
		opts.Format = "html"
	default:
		return fmt.Errorf("unknown --format %q (want text, markdown, or html)", opts.Format)
	}
	if opts.Format != "html" {
		if opts.OutFile != "" {
			return fmt.Errorf("--out only applies to --format=html")
		}
		if opts.Open {
			return fmt.Errorf("--open only applies to --format=html")
		}
		if opts.IncludeFollowUps {
			return fmt.Errorf("--include-follow-ups only applies to --format=html")
		}
	}
	if opts.Format == "html" && opts.OutFile == "-" && opts.Open {
		return fmt.Errorf("--open cannot be used with --out -")
	}
	return nil
}

func runChatShow(args []string) error {
	return runChatShowWithOptions(args, packageGlobalOptions())
}

func runChatShowWithOptions(args []string, globals globalOptions) error {
	opts, positional, err := parseChatShowArgsWithOptions(args, globals)
	if err != nil {
		return err
	}
	if len(positional) == 1 {
		return chatShowNotebook(positional[0], opts)
	}
	return chatShow(positional[0], positional[1], opts)
}

func validateCreateReportArgs(cmdName string, args []string) error {
	return validateCreateReportArgsWithOptions(cmdName, args, packageGlobalOptions())
}

func validateCreateReportArgsWithOptions(cmdName string, args []string, globals globalOptions) error {
	_, positional, err := parseCreateReportArgsWithOptions(args, globals)
	if err == nil && len(positional) >= 2 {
		return nil
	}
	fmt.Fprintf(os.Stderr, "usage: nlm %s <notebook-id> <report-type> [description] [instructions]\n", cmdName)
	return errBadArgs
}

func parseCreateReportArgs(args []string) (createReportOptions, []string, error) {
	return parseCreateReportArgsWithOptions(args, packageGlobalOptions())
}

func parseCreateReportArgsWithOptions(args []string, globals globalOptions) (createReportOptions, []string, error) {
	opts := createReportOptions{Selectors: selectorOptionsFromGlobals(globals)}
	flags := flag.NewFlagSet("create-report", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	appendSelectorFlags(flags, &opts.Selectors)

	flagArgs, positional, err := splitCommandFlags(args, selectorCommandLocalFlags(), nil)
	if err != nil {
		return opts, nil, err
	}
	if err := flags.Parse(flagArgs); err != nil {
		return opts, nil, err
	}
	if len(positional) < 2 {
		return opts, nil, fmt.Errorf("want notebook id and report type")
	}
	return opts, positional, nil
}

func runCreateReport(c *api.Client, args []string) error {
	return runCreateReportWithOptions(c, args, packageGlobalOptions())
}

func runCreateReportWithOptions(c *api.Client, args []string, globals globalOptions) error {
	opts, positional, err := parseCreateReportArgsWithOptions(args, globals)
	if err != nil {
		return err
	}
	return createReport(c, positional[0], positional[1], positional[2:], opts)
}

func printGenerateReportUsage(cmdName string) {
	fmt.Fprintf(os.Stderr, "Usage: nlm %s [flags] <notebook-id>\n\n", cmdName)
	fmt.Fprintln(os.Stderr, "Flags:")
	fmt.Fprintln(os.Stderr, "  --prompt <template>      Per-section prompt template ({topic} is replaced)")
	fmt.Fprintln(os.Stderr, "  --instructions <text>    Set notebook instructions before generation")
	fmt.Fprintln(os.Stderr, "  --sections <n>           Generate at most n sections (0 = all)")
	fmt.Fprintln(os.Stderr, "  --thinking, --reasoning  Show thinking headers while streaming")
	fmt.Fprintln(os.Stderr, "  --verbose, -v            Show full thinking traces while streaming")
	fmt.Fprintln(os.Stderr, "  --citations <mode>       Citation rendering: off|list|json (default list; block/stream/tail are deprecated aliases of list)")
	fmt.Fprintln(os.Stderr, "  --citation-confidence=off  Hide the (p=…) confidence column in the citation list")
	fmt.Fprintln(os.Stderr, "  --citation-spans=off       Hide the trailing [chars N-M] span column in the citation list")
	fmt.Fprintln(os.Stderr, "  --resolve-citations      Resolve citations to file:line for txtar-archive sources")
	fmt.Fprintln(os.Stderr, "  --citation-excerpts[=N]  Show the cited source text under each citation (N chars, default 160)")
	fmt.Fprintln(os.Stderr, "  --source-ids <ids>       Focus on these source IDs ('a,b,c' or '-' for stdin)")
	fmt.Fprintln(os.Stderr, "  --source-match <regex>   Focus on sources whose title or UUID matches the regex")
	fmt.Fprintln(os.Stderr, "  --source-exclude <regex> Exclude sources whose title or UUID matches the regex")
	fmt.Fprintln(os.Stderr, "  --label-ids <ids>        Include sources tagged with any of these label IDs")
	fmt.Fprintln(os.Stderr, "  --label-match <regex>    Include sources tagged with any label whose name matches the regex")
	fmt.Fprintln(os.Stderr, "  --label-exclude <regex>  Exclude sources tagged with any label whose name matches the regex")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Examples:")
	fmt.Fprintf(os.Stderr, "  nlm %s <notebook-id>\n", cmdName)
	fmt.Fprintf(os.Stderr, "  nlm %s --sections 3 <notebook-id>\n", cmdName)
	fmt.Fprintf(os.Stderr, "  nlm %s --prompt '# {topic}\\n\\nExplain the design.' <notebook-id>\n", cmdName)
}

func validateGenerateReportArgs(cmdName string, args []string) error {
	return validateGenerateReportArgsWithOptions(cmdName, args, packageGlobalOptions())
}

func validateGenerateReportArgsWithOptions(cmdName string, args []string, globals globalOptions) error {
	_, positional, err := parseGenerateReportArgsWithOptions(args, globals)
	if err == nil && len(positional) == 1 {
		return nil
	}
	fmt.Fprintf(os.Stderr, "usage: nlm %s <notebook-id>\n", cmdName)
	return errBadArgs
}

func parseGenerateReportArgs(args []string) (reportOptions, []string, error) {
	return parseGenerateReportArgsWithOptions(args, packageGlobalOptions())
}

func parseGenerateReportArgsWithOptions(args []string, globals globalOptions) (reportOptions, []string, error) {
	opts := reportOptions{
		Prompt:       globals.reportPrompt,
		Instructions: globals.reportInstructions,
		Sections:     globals.reportSections,
		Selectors:    selectorOptionsFromGlobals(globals),
		Render:       chatRenderOptionsFromGlobals(globals),
	}
	flags := flag.NewFlagSet("generate-report", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&opts.Prompt, "prompt", opts.Prompt, "")
	flags.StringVar(&opts.Instructions, "instructions", opts.Instructions, "")
	flags.IntVar(&opts.Sections, "sections", opts.Sections, "")
	appendRenderFlags(flags, &opts.Render)
	appendSelectorFlags(flags, &opts.Selectors)

	flagArgs, positional, err := splitCommandFlags(args, map[string]bool{
		"prompt":              true,
		"instructions":        true,
		"sections":            true,
		"thinking":            true,
		"reasoning":           true,
		"thinking-jsonl":      true,
		"verbose":             true,
		"v":                   true,
		"citations":           true,
		"resolve-citations":   true,
		"citation-excerpts":   true,
		"citation-excerpt":    true,
		"citation-confidence": true,
		"citation-spans":      true,
		"source-ids":          true,
		"source-match":        true,
		"source-exclude":      true,
		"label-ids":           true,
		"label-match":         true,
		"label-exclude":       true,
	}, map[string]bool{
		"thinking":            true,
		"reasoning":           true,
		"thinking-jsonl":      true,
		"verbose":             true,
		"v":                   true,
		"resolve-citations":   true,
		"citation-excerpts":   true,
		"citation-excerpt":    true,
		"citation-confidence": true,
		"citation-spans":      true,
	})
	if err != nil {
		return opts, nil, err
	}
	if err := flags.Parse(flagArgs); err != nil {
		return opts, nil, err
	}
	if len(positional) != 1 {
		return opts, nil, fmt.Errorf("want notebook id")
	}
	if opts.Sections < 0 {
		return opts, nil, fmt.Errorf("--sections must be >= 0")
	}
	return opts, positional, nil
}

func runGenerateReport(c *api.Client, args []string) error {
	return runGenerateReportWithOptions(c, args, packageGlobalOptions())
}

func runGenerateReportWithOptions(c *api.Client, args []string, globals globalOptions) error {
	opts, positional, err := parseGenerateReportArgsWithOptions(args, globals)
	if err != nil {
		return err
	}
	return generateReport(c, positional[0], opts)
}
