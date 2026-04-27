package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/tmc/nlm/internal/notebooklm/api"
)

type selectorOptions struct {
	SourceIDs     string
	SourceMatch   string
	SourceExclude string
	LabelIDs      string
	LabelMatch    string
	LabelExclude  string
}

func currentSelectorOptions() selectorOptions {
	return selectorOptions{
		SourceIDs:     sourceIDsFlag,
		SourceMatch:   sourceMatchFlag,
		SourceExclude: sourceExcludeFlag,
		LabelIDs:      labelIDsFlag,
		LabelMatch:    labelMatchFlag,
		LabelExclude:  labelExcludeFlag,
	}
}

func (opts selectorOptions) empty() bool {
	return opts.SourceIDs == "" &&
		opts.SourceMatch == "" &&
		opts.SourceExclude == "" &&
		opts.LabelIDs == "" &&
		opts.LabelMatch == "" &&
		opts.LabelExclude == ""
}

// resolveSourceSelectors returns the union of source IDs from --source-ids,
// --source-match, and label-include selectors, minus the source-exclude
// regex and label-exclude matches. Returns nil when no selectors are set —
// callers decide whether that's "no scoping" (use all) or an error.
//
// Fails hard when a regex selector is set but resolves to nothing,
// listing the available sources/labels on stderr.
func resolveSourceSelectors(c *api.Client, notebookID string) ([]string, error) {
	return resolveSourceSelectorsWithOptions(c, notebookID, currentSelectorOptions())
}

func resolveSourceSelectorsWithOptions(c *api.Client, notebookID string, opts selectorOptions) ([]string, error) {
	flagIDs, err := resolveIDList(opts.SourceIDs)
	if err != nil {
		return nil, fmt.Errorf("--source-ids: %w", err)
	}
	flagLabelIDs, err := resolveIDList(opts.LabelIDs)
	if err != nil {
		return nil, fmt.Errorf("--label-ids: %w", err)
	}

	needsLabels := len(flagLabelIDs) > 0 || opts.LabelMatch != "" || opts.LabelExclude != ""
	needsSources := opts.SourceMatch != "" || opts.SourceExclude != "" || needsLabels

	var labels []api.Label
	var sources []sourceSummary
	if needsSources {
		p, perr := c.GetProject(notebookID)
		if perr != nil {
			return nil, fmt.Errorf("list sources for selectors: %w", perr)
		}
		sources = make([]sourceSummary, 0, len(p.Sources))
		for _, src := range p.Sources {
			sources = append(sources, sourceSummary{
				ID:    src.SourceId.GetSourceId(),
				Title: strings.TrimSpace(src.Title),
			})
		}
	}
	if needsLabels {
		ls, lerr := c.GetLabels(notebookID)
		if lerr != nil {
			return nil, fmt.Errorf("list labels for selectors: %w", lerr)
		}
		labels = ls
	}

	return resolveSelectorIDs(opts, flagIDs, flagLabelIDs, sources, labels, os.Stderr)
}

// sourceSummary is the projection of a source needed by selector resolution.
// Decoupled from pb.Source so resolveSelectorIDs is unit-testable without
// constructing protobufs or mocking the API client.
type sourceSummary struct {
	ID    string
	Title string
}

// resolveSelectorIDs is the pure resolution logic. statusW receives the
// human-readable explanations (one line per active selector). Returns the
// final ID list with order-preserved de-duplication.
func resolveSelectorIDs(opts selectorOptions, flagIDs, flagLabelIDs []string, sources []sourceSummary, labels []api.Label, statusW interface{ Write([]byte) (int, error) }) ([]string, error) {
	if opts.empty() {
		return nil, nil
	}
	sourceMatchRE, err := compileSelectorRegex("--source-match", opts.SourceMatch)
	if err != nil {
		return nil, err
	}
	sourceExcludeRE, err := compileSelectorRegex("--source-exclude", opts.SourceExclude)
	if err != nil {
		return nil, err
	}
	labelMatchRE, err := compileSelectorRegex("--label-match", opts.LabelMatch)
	if err != nil {
		return nil, err
	}
	labelExcludeRE, err := compileSelectorRegex("--label-exclude", opts.LabelExclude)
	if err != nil {
		return nil, err
	}

	includeAll := len(flagIDs) == 0 &&
		len(flagLabelIDs) == 0 &&
		sourceMatchRE == nil &&
		labelMatchRE == nil
	// When only excludes are set, the include set is "all known sources".
	hasOnlyExcludes := includeAll && (sourceExcludeRE != nil || labelExcludeRE != nil)

	includeSet := make(map[string]bool)
	var includeOrder []string
	add := func(id string) {
		if id == "" || includeSet[id] {
			return
		}
		includeSet[id] = true
		includeOrder = append(includeOrder, id)
	}

	if hasOnlyExcludes {
		for _, s := range sources {
			add(s.ID)
		}
	} else if !includeAll {
		for _, id := range flagIDs {
			add(id)
		}
		if sourceMatchRE != nil {
			matched := matchSources(sources, sourceMatchRE)
			if len(matched) == 0 {
				listAvailableSources(statusW, "--source-match", opts.SourceMatch, sources)
				return nil, fmt.Errorf("--source-match matched no sources")
			}
			fmt.Fprintf(statusW, "--source-match %q: %d source(s)\n", opts.SourceMatch, len(matched))
			for _, m := range matched {
				fmt.Fprintf(statusW, "  %s\n", m.Title)
				add(m.ID)
			}
		}
		if len(flagLabelIDs) > 0 || labelMatchRE != nil {
			labelHits := matchLabels(labels, flagLabelIDs, labelMatchRE)
			if len(labelHits) == 0 && (len(flagLabelIDs) > 0 || labelMatchRE != nil) {
				listAvailableLabels(statusW, opts, labels)
				return nil, fmt.Errorf("label selectors matched no labels")
			}
			for _, l := range labelHits {
				fmt.Fprintf(statusW, "label %q (%s): %d source(s)\n", l.Name, l.LabelID, len(l.SourceIDs))
				for _, id := range l.SourceIDs {
					add(id)
				}
			}
		}
	}

	excludeIDs := make(map[string]bool)
	if sourceExcludeRE != nil {
		excluded := matchSources(sources, sourceExcludeRE)
		fmt.Fprintf(statusW, "--source-exclude %q: %d source(s)\n", opts.SourceExclude, len(excluded))
		for _, e := range excluded {
			excludeIDs[e.ID] = true
		}
	}
	if labelExcludeRE != nil {
		excludedLabels := matchLabels(labels, nil, labelExcludeRE)
		for _, l := range excludedLabels {
			fmt.Fprintf(statusW, "--label-exclude %q matched label %q: %d source(s)\n", opts.LabelExclude, l.Name, len(l.SourceIDs))
			for _, id := range l.SourceIDs {
				excludeIDs[id] = true
			}
		}
	}

	if len(excludeIDs) == 0 {
		return includeOrder, nil
	}
	out := make([]string, 0, len(includeOrder))
	for _, id := range includeOrder {
		if excludeIDs[id] {
			continue
		}
		out = append(out, id)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("selectors resolved to empty set after exclusions")
	}
	return out, nil
}

func compileSelectorRegex(flag, expr string) (*regexp.Regexp, error) {
	if expr == "" {
		return nil, nil
	}
	re, err := regexp.Compile(expr)
	if err != nil {
		return nil, fmt.Errorf("%s: invalid regex: %w", flag, err)
	}
	return re, nil
}

func matchSources(sources []sourceSummary, re *regexp.Regexp) []sourceSummary {
	var out []sourceSummary
	for _, s := range sources {
		if re.MatchString(s.Title) || re.MatchString(s.ID) {
			out = append(out, s)
		}
	}
	return out
}

// matchLabels returns labels whose label_id is in includeIDs OR whose name
// matches re. If both filters are empty, returns nil.
func matchLabels(labels []api.Label, includeIDs []string, re *regexp.Regexp) []api.Label {
	idSet := make(map[string]bool, len(includeIDs))
	for _, id := range includeIDs {
		idSet[id] = true
	}
	var out []api.Label
	for _, l := range labels {
		if idSet[l.LabelID] || (re != nil && re.MatchString(l.Name)) {
			out = append(out, l)
		}
	}
	return out
}

func listAvailableSources(w interface{ Write([]byte) (int, error) }, flag, expr string, sources []sourceSummary) {
	fmt.Fprintf(w, "%s %q matched no sources. Available titles:\n", flag, expr)
	for _, s := range sources {
		fmt.Fprintf(w, "  %s\n", s.Title)
	}
}

func listAvailableLabels(w interface{ Write([]byte) (int, error) }, opts selectorOptions, labels []api.Label) {
	switch {
	case opts.LabelMatch != "" && len(opts.LabelIDs) > 0:
		fmt.Fprintf(w, "--label-ids/--label-match matched no labels. Available labels:\n")
	case opts.LabelMatch != "":
		fmt.Fprintf(w, "--label-match %q matched no labels. Available labels:\n", opts.LabelMatch)
	default:
		fmt.Fprintf(w, "--label-ids matched no labels. Available labels:\n")
	}
	for _, l := range labels {
		fmt.Fprintf(w, "  %s (%s)\n", l.Name, l.LabelID)
	}
}
