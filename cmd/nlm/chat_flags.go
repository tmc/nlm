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
	ShowThinking  bool
	ThinkingJSONL bool
	Verbose       bool
	CitationMode  string
}

type generateChatOptions struct {
	ConversationID string
	UseWebChat     bool
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

func currentChatRenderOptions() chatRenderOptions {
	return chatRenderOptions{
		ShowThinking:  showThinking,
		ThinkingJSONL: thinkingJSONL,
		Verbose:       verbose,
		CitationMode:  citationMode,
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
}

func printGenerateChatUsage(cmdName string) {
	fmt.Fprintf(os.Stderr, "Usage: nlm %s [flags] <notebook-id> <prompt>\n\n", cmdName)
	fmt.Fprintln(os.Stderr, "Flags:")
	fmt.Fprintln(os.Stderr, "  --conversation, -c <id>  Continue an existing conversation by ID")
	fmt.Fprintln(os.Stderr, "  --web                    Use the most recent server-side conversation")
	fmt.Fprintln(os.Stderr, "  --thinking, --reasoning  Show thinking headers while streaming")
	fmt.Fprintln(os.Stderr, "  --thinking-jsonl         Emit thinking/answer/citation JSON-lines events")
	fmt.Fprintln(os.Stderr, "  --verbose, -v            Show full thinking traces while streaming")
	fmt.Fprintln(os.Stderr, "  --citations <mode>       Citation rendering: off|block|stream|tail|overlay|json")
	fmt.Fprintln(os.Stderr, "  --source-ids <ids>       Focus on these source IDs ('a,b,c' or '-' for stdin)")
	fmt.Fprintln(os.Stderr, "  --source-match <regex>   Focus on sources whose title or UUID matches the regex")
	fmt.Fprintln(os.Stderr, "  --source-exclude <regex> Exclude sources whose title or UUID matches the regex")
	fmt.Fprintln(os.Stderr, "  --label-ids <ids>        Include sources tagged with any of these label IDs")
	fmt.Fprintln(os.Stderr, "  --label-match <regex>    Include sources tagged with any label whose name matches the regex")
	fmt.Fprintln(os.Stderr, "  --label-exclude <regex>  Exclude sources tagged with any label whose name matches the regex")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Examples:")
	fmt.Fprintf(os.Stderr, "  nlm %s <notebook-id> \"Summarize the architecture\"\n", cmdName)
	fmt.Fprintf(os.Stderr, "  nlm %s --conversation <id> <notebook-id> \"Follow up on section 2\"\n", cmdName)
}

func validateGenerateChatArgs(cmdName string, args []string) error {
	_, positional, err := parseGenerateChatArgs(args)
	if err == nil && len(positional) >= 2 {
		return nil
	}
	fmt.Fprintf(os.Stderr, "usage: nlm %s <notebook-id> <prompt>\n", cmdName)
	return errBadArgs
}

func parseGenerateChatArgs(args []string) (generateChatOptions, []string, error) {
	opts := generateChatOptions{
		ConversationID: conversationID,
		UseWebChat:     useWebChat,
		Selectors:      currentSelectorOptions(),
		Render:         currentChatRenderOptions(),
	}
	flags := flag.NewFlagSet("generate-chat", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&opts.ConversationID, "conversation", opts.ConversationID, "")
	flags.StringVar(&opts.ConversationID, "c", opts.ConversationID, "")
	flags.BoolVar(&opts.UseWebChat, "web", opts.UseWebChat, "")
	appendRenderFlags(flags, &opts.Render)
	appendSelectorFlags(flags, &opts.Selectors)

	flagArgs, positional, err := splitCommandFlags(args, map[string]bool{
		"conversation":   true,
		"c":              true,
		"web":            true,
		"thinking":       true,
		"reasoning":      true,
		"thinking-jsonl": true,
		"verbose":        true,
		"v":              true,
		"citations":      true,
		"source-ids":     true,
		"source-match":   true,
		"source-exclude": true,
		"label-ids":      true,
		"label-match":    true,
		"label-exclude":  true,
	}, map[string]bool{
		"web":            true,
		"thinking":       true,
		"reasoning":      true,
		"thinking-jsonl": true,
		"verbose":        true,
		"v":              true,
	})
	if err != nil {
		return opts, nil, err
	}
	if err := flags.Parse(flagArgs); err != nil {
		return opts, nil, err
	}
	if len(positional) < 2 {
		return opts, nil, fmt.Errorf("missing notebook id or prompt")
	}
	return opts, positional, nil
}

func runGenerateChat(c *api.Client, args []string) error {
	opts, positional, err := parseGenerateChatArgs(args)
	if err != nil {
		return err
	}
	return generateFreeFormChat(c, positional[0], strings.Join(positional[1:], " "), opts)
}

func printChatUsage(cmdName string) {
	fmt.Fprintf(os.Stderr, "Usage: nlm %s [flags] <notebook-id> [conversation-id | prompt]\n\n", cmdName)
	fmt.Fprintln(os.Stderr, "Flags:")
	fmt.Fprintln(os.Stderr, "  --prompt-file, -f <path> Read the prompt from a file ('-' reads stdin)")
	fmt.Fprintln(os.Stderr, "  --history                Show previous chat conversation on start")
	fmt.Fprintln(os.Stderr, "  --thinking, --reasoning  Show thinking headers while streaming")
	fmt.Fprintln(os.Stderr, "  --thinking-jsonl         Emit thinking/answer/citation JSON-lines events")
	fmt.Fprintln(os.Stderr, "  --verbose, -v            Show full thinking traces while streaming")
	fmt.Fprintln(os.Stderr, "  --citations <mode>       Citation rendering: off|block|stream|tail|overlay|json")
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
	_, positional, err := parseChatArgs(args)
	if err == nil && len(positional) >= 1 {
		return nil
	}
	fmt.Fprintf(os.Stderr, "usage: nlm %s <notebook-id> [conversation-id | prompt]\n", cmdName)
	return errBadArgs
}

func parseChatArgs(args []string) (chatOptions, []string, error) {
	opts := chatOptions{
		PromptFile:  promptFile,
		ShowHistory: showChatHistory,
		Selectors:   currentSelectorOptions(),
		Render:      currentChatRenderOptions(),
	}
	flags := flag.NewFlagSet("chat", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&opts.PromptFile, "prompt-file", opts.PromptFile, "")
	flags.StringVar(&opts.PromptFile, "f", opts.PromptFile, "")
	flags.BoolVar(&opts.ShowHistory, "history", opts.ShowHistory, "")
	appendRenderFlags(flags, &opts.Render)
	appendSelectorFlags(flags, &opts.Selectors)

	flagArgs, positional, err := splitCommandFlags(args, map[string]bool{
		"prompt-file":    true,
		"f":              true,
		"history":        true,
		"thinking":       true,
		"reasoning":      true,
		"thinking-jsonl": true,
		"verbose":        true,
		"v":              true,
		"citations":      true,
		"source-ids":     true,
		"source-match":   true,
		"source-exclude": true,
		"label-ids":      true,
		"label-match":    true,
		"label-exclude":  true,
	}, map[string]bool{
		"history":        true,
		"thinking":       true,
		"reasoning":      true,
		"thinking-jsonl": true,
		"verbose":        true,
		"v":              true,
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
	opts, positional, err := parseChatArgs(args)
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
	fmt.Fprintf(os.Stderr, "Usage: nlm %s [flags] <notebook-id> <conversation-id>\n\n", cmdName)
	fmt.Fprintln(os.Stderr, "Flags:")
	fmt.Fprintln(os.Stderr, "  --thinking, --reasoning  Show persisted thinking traces on stderr")
	fmt.Fprintln(os.Stderr, "  --citations <mode>       Citation rendering: off|block|stream|tail|overlay|json")
}

func validateChatShowArgs(cmdName string, args []string) error {
	_, positional, err := parseChatShowArgs(args)
	if err == nil && len(positional) == 2 {
		return nil
	}
	fmt.Fprintf(os.Stderr, "usage: nlm %s <notebook-id> <conversation-id>\n", cmdName)
	return errBadArgs
}

func parseChatShowArgs(args []string) (chatRenderOptions, []string, error) {
	opts := currentChatRenderOptions()
	flags := flag.NewFlagSet("chat-show", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.BoolVar(&opts.ShowThinking, "thinking", opts.ShowThinking, "")
	flags.BoolVar(&opts.ShowThinking, "reasoning", opts.ShowThinking, "")
	flags.StringVar(&opts.CitationMode, "citations", opts.CitationMode, "")

	flagArgs, positional, err := splitCommandFlags(args, map[string]bool{
		"thinking":  true,
		"reasoning": true,
		"citations": true,
	}, map[string]bool{
		"thinking":  true,
		"reasoning": true,
	})
	if err != nil {
		return opts, nil, err
	}
	if err := flags.Parse(flagArgs); err != nil {
		return opts, nil, err
	}
	if len(positional) != 2 {
		return opts, nil, fmt.Errorf("want notebook id and conversation id")
	}
	return opts, positional, nil
}

func runChatShow(args []string) error {
	opts, positional, err := parseChatShowArgs(args)
	if err != nil {
		return err
	}
	return chatShow(positional[0], positional[1], opts)
}

func printGenerateReportUsage(cmdName string) {
	fmt.Fprintf(os.Stderr, "Usage: nlm %s [flags] <notebook-id>\n\n", cmdName)
	fmt.Fprintln(os.Stderr, "Flags:")
	fmt.Fprintln(os.Stderr, "  --prompt <template>      Per-section prompt template ({topic} is replaced)")
	fmt.Fprintln(os.Stderr, "  --instructions <text>    Set notebook instructions before generation")
	fmt.Fprintln(os.Stderr, "  --sections <n>           Generate at most n sections (0 = all)")
	fmt.Fprintln(os.Stderr, "  --thinking, --reasoning  Show thinking headers while streaming")
	fmt.Fprintln(os.Stderr, "  --thinking-jsonl         Emit thinking/answer/citation JSON-lines events")
	fmt.Fprintln(os.Stderr, "  --verbose, -v            Show full thinking traces while streaming")
	fmt.Fprintln(os.Stderr, "  --citations <mode>       Citation rendering: off|block|stream|tail|overlay|json")
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
	_, positional, err := parseGenerateReportArgs(args)
	if err == nil && len(positional) == 1 {
		return nil
	}
	fmt.Fprintf(os.Stderr, "usage: nlm %s <notebook-id>\n", cmdName)
	return errBadArgs
}

func parseGenerateReportArgs(args []string) (reportOptions, []string, error) {
	opts := reportOptions{
		Prompt:       reportPrompt,
		Instructions: reportInstructions,
		Sections:     reportSections,
		Selectors:    currentSelectorOptions(),
		Render:       currentChatRenderOptions(),
	}
	flags := flag.NewFlagSet("generate-report", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&opts.Prompt, "prompt", opts.Prompt, "")
	flags.StringVar(&opts.Instructions, "instructions", opts.Instructions, "")
	flags.IntVar(&opts.Sections, "sections", opts.Sections, "")
	appendRenderFlags(flags, &opts.Render)
	appendSelectorFlags(flags, &opts.Selectors)

	flagArgs, positional, err := splitCommandFlags(args, map[string]bool{
		"prompt":         true,
		"instructions":   true,
		"sections":       true,
		"thinking":       true,
		"reasoning":      true,
		"thinking-jsonl": true,
		"verbose":        true,
		"v":              true,
		"citations":      true,
		"source-ids":     true,
		"source-match":   true,
		"source-exclude": true,
		"label-ids":      true,
		"label-match":    true,
		"label-exclude":  true,
	}, map[string]bool{
		"thinking":       true,
		"reasoning":      true,
		"thinking-jsonl": true,
		"verbose":        true,
		"v":              true,
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
	opts, positional, err := parseGenerateReportArgs(args)
	if err != nil {
		return err
	}
	return generateReport(c, positional[0], opts)
}
