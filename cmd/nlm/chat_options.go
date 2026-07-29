package main

import (
	"fmt"
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

	Client commandClientOptions
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
	Yes         bool
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
