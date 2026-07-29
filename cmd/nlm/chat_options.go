package main

import (
	"fmt"
	"os"
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

func (o chatRenderOptions) jsonl() bool {
	return o.ThinkingJSONL || resolveCitationMode(o.CitationMode) == citationModeJSON
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
