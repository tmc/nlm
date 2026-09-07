package main

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/tmc/nlm/notebooklm"
)

// selection distinguishes an omitted scope from an explicit source subset.
type selection struct {
	Explicit bool
	IDs      []string
}

// sourceIDs adapts a CLI selection to the library's empty-means-all contract.
func (s selection) sourceIDs() ([]string, error) {
	if !s.Explicit {
		return nil, nil
	}
	if len(s.IDs) == 0 {
		return nil, fmt.Errorf("selectors resolved to empty set")
	}
	return s.IDs, nil
}

type selectorOptions struct {
	Mode            string
	LabelNone       bool
	LabelExcludeIDs string
	SourceIDs       string
	SourceMatch     string
	SourceExclude   string
	LabelIDs        string
	LabelMatch      string
	LabelExclude    string
}

func selectorOptionsFromGlobals(globals globalOptions) selectorOptions {
	return selectorOptions{
		SourceIDs:     globals.sourceIDsFlag,
		SourceMatch:   globals.sourceMatchFlag,
		SourceExclude: globals.sourceExcludeFlag,
		LabelIDs:      globals.labelIDsFlag,
		LabelMatch:    globals.labelMatchFlag,
		LabelExclude:  globals.labelExcludeFlag,
	}
}

func (opts selectorOptions) empty() bool {
	return !opts.LabelNone && opts.LabelExcludeIDs == "" && opts.Mode == "" && opts.SourceIDs == "" &&
		opts.SourceMatch == "" &&
		opts.SourceExclude == "" &&
		opts.LabelIDs == "" &&
		opts.LabelMatch == "" &&
		opts.LabelExclude == ""
}

// validateStdin reserves stdin for at most one input before any reader runs.
func (opts selectorOptions) validateStdin(other bool) error {
	n := 0
	if other {
		n++
	}
	for _, value := range []string{opts.SourceIDs, opts.LabelIDs, opts.LabelExcludeIDs} {
		if value == "-" {
			n++
		}
	}
	if n > 1 {
		return fmt.Errorf("at most one stdin consumer is allowed")
	}
	return nil
}

// validate checks input shape before stdin or RPCs are touched.
func (opts selectorOptions) validate() error {
	if err := opts.validateStdin(false); err != nil {
		return err
	}
	withoutMode := opts
	withoutMode.Mode = ""
	if opts.Mode != "" && withoutMode.empty() {
		return fmt.Errorf("--selector-mode requires selectors")
	}
	switch opts.Mode {
	case "", "union":
	case "intersect":
		return fmt.Errorf("--selector-mode=intersect is not available yet")
	default:
		return fmt.Errorf("invalid --selector-mode %q", opts.Mode)
	}
	if opts.Mode == "" && (opts.SourceIDs != "" || opts.SourceMatch != "") && (opts.LabelIDs != "" || opts.LabelMatch != "" || opts.LabelNone) {
		return fmt.Errorf("mixed source and label includes require --selector-mode: union combines matches; intersect narrows to both (not available yet)")
	}
	for _, flag := range []struct{ name, expr string }{
		{"--source-match", opts.SourceMatch}, {"--source-exclude", opts.SourceExclude},
		{"--label-match", opts.LabelMatch}, {"--label-exclude", opts.LabelExclude},
	} {
		if _, err := compileSelectorRegex(flag.name, flag.expr); err != nil {
			return err
		}
	}
	return nil
}

func resolveSourceSelectorsWithOptions(c *notebooklm.Client, notebookID string, opts selectorOptions) (selection, error) {
	if err := opts.validate(); err != nil {
		return selection{}, err
	}
	flagIDs, err := resolveIDList(opts.SourceIDs)
	if err != nil {
		return selection{}, fmt.Errorf("--source-ids: %w", err)
	}
	flagLabelIDs, err := resolveIDList(opts.LabelIDs)
	if err != nil {
		return selection{}, fmt.Errorf("--label-ids: %w", err)
	}

	flagLabelExcludeIDs, err := resolveIDList(opts.LabelExcludeIDs)
	if err != nil {
		return selection{}, fmt.Errorf("--label-exclude-ids: %w", err)
	}
	// An empty explicit ID input alone cannot select anything, even on a
	// nonempty notebook. Reject before fetching the source universe.
	if opts.SourceIDs != "" && len(flagIDs) == 0 && opts.SourceMatch == "" && opts.LabelIDs == "" && opts.LabelMatch == "" && !opts.LabelNone {
		return selection{}, fmt.Errorf("--source-ids: selectors resolved to empty set")
	}
	needsLabels := opts.LabelIDs != "" || opts.LabelMatch != "" || opts.LabelExclude != "" || opts.LabelExcludeIDs != "" || opts.LabelNone
	needsSources := !opts.empty()

	var labels []notebooklm.Label
	var sources []sourceSummary
	if needsSources {
		p, perr := c.GetProject(context.Background(), notebookID)
		if perr != nil {
			return selection{}, fmt.Errorf("list sources for selectors: %w", perr)
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
		ls, lerr := c.GetLabels(context.Background(), notebookID)
		if lerr != nil {
			return selection{}, fmt.Errorf("list labels for selectors: %w", lerr)
		}
		labels = ls
	}

	return resolveSelectorIDs(opts, flagIDs, flagLabelIDs, flagLabelExcludeIDs, sources, labels, os.Stderr)
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
func resolveSelectorIDs(opts selectorOptions, flagIDs, flagLabelIDs, flagLabelExcludeIDs []string, sources []sourceSummary, labels []notebooklm.Label, statusW interface{ Write([]byte) (int, error) }) (selection, error) {
	if err := opts.validate(); err != nil {
		return selection{}, err
	}
	if opts.empty() {
		return selection{}, nil
	}
	sourceMatchRE, err := compileSelectorRegex("--source-match", opts.SourceMatch)
	if err != nil {
		return selection{}, err
	}
	sourceExcludeRE, err := compileSelectorRegex("--source-exclude", opts.SourceExclude)
	if err != nil {
		return selection{}, err
	}
	labelMatchRE, err := compileSelectorRegex("--label-match", opts.LabelMatch)
	if err != nil {
		return selection{}, err
	}
	labelExcludeRE, err := compileSelectorRegex("--label-exclude", opts.LabelExclude)
	if err != nil {
		return selection{}, err
	}

	knownSources := make(map[string]bool)
	for _, source := range sources {
		knownSources[source.ID] = true
	}
	knownLabels := make(map[string]bool)
	for _, label := range labels {
		knownLabels[label.LabelID] = true
	}
	for _, input := range []struct {
		name  string
		ids   []string
		known map[string]bool
	}{
		{"--source-ids", flagIDs, knownSources},
		{"--label-ids", flagLabelIDs, knownLabels},
		{"--label-exclude-ids", flagLabelExcludeIDs, knownLabels},
	} {
		var unknown []string
		for _, id := range input.ids {
			if !input.known[id] {
				unknown = append(unknown, id)
			}
		}
		if len(unknown) > 0 {
			return selection{}, fmt.Errorf("%s: unknown IDs: %s", input.name, strings.Join(unknown, ", "))
		}
	}

	includeAll := opts.SourceIDs == "" &&
		opts.LabelIDs == "" &&
		sourceMatchRE == nil &&
		labelMatchRE == nil && !opts.LabelNone
	// When only excludes are set, the include set is "all known sources".
	hasOnlyExcludes := includeAll && (sourceExcludeRE != nil || labelExcludeRE != nil || opts.LabelExcludeIDs != "")

	includeSet := make(map[string]bool)
	var includeOrder []string
	add := func(id string) {
		if id == "" || !knownSources[id] || includeSet[id] {
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

			fmt.Fprintf(statusW, "--source-match %q: %d source(s)\n", opts.SourceMatch, len(matched))
			for _, m := range matched {
				fmt.Fprintf(statusW, "  %s\n", m.Title)
				add(m.ID)
			}
		}
		if len(flagLabelIDs) > 0 || labelMatchRE != nil {
			labelHits := matchLabels(labels, flagLabelIDs, labelMatchRE)

			for _, l := range labelHits {
				fmt.Fprintf(statusW, "label %q (%s): %d source(s)\n", l.Name, l.LabelID, len(l.SourceIDs))
				for _, id := range l.SourceIDs {
					add(id)
				}
			}
		}
	}

	if opts.LabelNone {
		labeled := make(map[string]bool)
		for _, label := range labels {
			for _, id := range label.SourceIDs {
				labeled[id] = true
			}
		}
		for _, source := range sources {
			if !labeled[source.ID] {
				add(source.ID)
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
	if labelExcludeRE != nil || len(flagLabelExcludeIDs) > 0 {
		excludedLabels := matchLabels(labels, flagLabelExcludeIDs, labelExcludeRE)
		for _, l := range excludedLabels {
			fmt.Fprintf(statusW, "--label-exclude %q matched label %q: %d source(s)\n", opts.LabelExclude, l.Name, len(l.SourceIDs))
			for _, id := range l.SourceIDs {
				excludeIDs[id] = true
			}
		}
	}

	if len(excludeIDs) == 0 {
		if len(includeOrder) == 0 {
			return selection{}, fmt.Errorf("selectors resolved to empty set")
		}
		return selection{Explicit: true, IDs: includeOrder}, nil
	}
	out := make([]string, 0, len(includeOrder))
	for _, id := range includeOrder {
		if excludeIDs[id] {
			continue
		}
		out = append(out, id)
	}
	if len(out) == 0 {
		return selection{}, fmt.Errorf("selectors resolved to empty set after exclusions")
	}
	return selection{Explicit: true, IDs: out}, nil
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
func matchLabels(labels []notebooklm.Label, includeIDs []string, re *regexp.Regexp) []notebooklm.Label {
	idSet := make(map[string]bool, len(includeIDs))
	for _, id := range includeIDs {
		idSet[id] = true
	}
	var out []notebooklm.Label
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

func listAvailableLabels(w interface{ Write([]byte) (int, error) }, opts selectorOptions, labels []notebooklm.Label) {
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
