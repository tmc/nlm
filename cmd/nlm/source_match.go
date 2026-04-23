package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/tmc/nlm/internal/notebooklm/api"
)

type selectorOptions struct {
	SourceIDs   string
	SourceMatch string
}

func currentSelectorOptions() selectorOptions {
	return selectorOptions{
		SourceIDs:   sourceIDsFlag,
		SourceMatch: sourceMatchFlag,
	}
}

func (opts selectorOptions) empty() bool {
	return opts.SourceIDs == "" && opts.SourceMatch == ""
}

// resolveSourceSelectors returns the union of source IDs from --source-ids
// and --source-match (regex against each source's title and UUID prefix).
// Returns nil when both flags are empty — callers decide whether that's
// "no scoping" (use all) or an error.
//
// Fails hard when --source-match is set but matches nothing, listing
// the available sources on stderr so the user can correct the regex.
func resolveSourceSelectors(c *api.Client, notebookID string) ([]string, error) {
	return resolveSourceSelectorsWithOptions(c, notebookID, currentSelectorOptions())
}

func resolveSourceSelectorsWithOptions(c *api.Client, notebookID string, opts selectorOptions) ([]string, error) {
	flagIDs, err := resolveIDList(opts.SourceIDs)
	if err != nil {
		return nil, fmt.Errorf("--source-ids: %w", err)
	}
	if opts.SourceMatch == "" {
		return flagIDs, nil
	}

	re, err := regexp.Compile(opts.SourceMatch)
	if err != nil {
		return nil, fmt.Errorf("--source-match: invalid regex: %w", err)
	}

	p, err := c.GetProject(notebookID)
	if err != nil {
		return nil, fmt.Errorf("list sources for --source-match: %w", err)
	}

	var matched []string
	var matchedTitles []string
	for _, src := range p.Sources {
		id := src.SourceId.GetSourceId()
		title := strings.TrimSpace(src.Title)
		if re.MatchString(title) || re.MatchString(id) {
			matched = append(matched, id)
			matchedTitles = append(matchedTitles, title)
		}
	}

	if len(matched) == 0 {
		fmt.Fprintf(os.Stderr, "--source-match %q matched no sources. Available titles:\n", opts.SourceMatch)
		for _, src := range p.Sources {
			fmt.Fprintf(os.Stderr, "  %s\n", strings.TrimSpace(src.Title))
		}
		return nil, fmt.Errorf("--source-match matched no sources")
	}

	fmt.Fprintf(os.Stderr, "--source-match %q: %d source(s)\n", opts.SourceMatch, len(matched))
	for _, t := range matchedTitles {
		fmt.Fprintf(os.Stderr, "  %s\n", t)
	}

	return unionIDs(flagIDs, matched), nil
}
