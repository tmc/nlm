package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/tmc/nlm/notebooklm"
)

// selection distinguishes an omitted scope from an explicit source subset.
type selection struct {
	Explicit bool
	IDs      []string
	excluded map[string]bool
	known    map[string]bool
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

// withSuggestions composes report inputs using the same exclusion snapshot
// as resolution. Suggestion IDs cannot restore a source explicitly excluded.
func (s selection) withSuggestions(ids []string) ([]string, error) {
	if _, err := s.sourceIDs(); err != nil {
		return nil, err
	}
	out := selection{Explicit: s.Explicit}
	for _, id := range unionIDs(s.IDs, ids) {
		if s.excluded[id] || (s.known != nil && !s.known[id]) {
			continue
		}
		out.IDs = append(out.IDs, id)
	}
	if !s.Explicit {
		return out.IDs, nil
	}
	return out.sourceIDs()
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
	case "", "union", "intersect":
	default:
		return fmt.Errorf("invalid --selector-mode %q", opts.Mode)
	}
	if opts.Mode == "" && (opts.SourceIDs != "" || opts.SourceMatch != "") && (opts.LabelIDs != "" || opts.LabelMatch != "" || opts.LabelNone) {
		return fmt.Errorf("mixed source and label includes require --selector-mode: union combines matches; intersect narrows to both")
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

	// The two snapshots are independent, and each costs a round trip that
	// runs for seconds on a large notebook, so fetch them together.
	type labelFetch struct {
		labels []notebooklm.Label
		err    error
	}
	labelCh := make(chan labelFetch, 1)
	if needsLabels {
		go func() {
			ls, lerr := c.GetLabels(context.Background(), notebookID)
			labelCh <- labelFetch{ls, lerr}
		}()
	}

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
		got := <-labelCh
		if got.err != nil {
			return selection{}, fmt.Errorf("list labels for selectors: %w", got.err)
		}
		labels = got.labels
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

// resolveSelectorIDs resolves against one source and label snapshot. Results
// follow source order, and only a final empty selection is an error.
func resolveSelectorIDs(opts selectorOptions, flagIDs, flagLabelIDs, flagLabelExcludeIDs []string, sources []sourceSummary, labels []notebooklm.Label, statusW io.Writer) (selection, error) {
	if err := opts.validate(); err != nil {
		return selection{}, err
	}
	if opts.empty() {
		return selection{}, nil
	}
	sourceMatchRE, _ := compileSelectorRegex("--source-match", opts.SourceMatch)
	sourceExcludeRE, _ := compileSelectorRegex("--source-exclude", opts.SourceExclude)
	labelMatchRE, _ := compileSelectorRegex("--label-match", opts.LabelMatch)
	labelExcludeRE, _ := compileSelectorRegex("--label-exclude", opts.LabelExclude)

	knownSources := make(map[string]bool)
	for _, source := range sources {
		if source.ID != "" {
			knownSources[source.ID] = true
		}
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

	sourceActive := opts.SourceIDs != "" || opts.SourceMatch != ""
	labelActive := opts.LabelIDs != "" || opts.LabelMatch != "" || opts.LabelNone
	sourceSet := make(map[string]bool)
	for _, id := range flagIDs {
		sourceSet[id] = true
	}
	if sourceMatchRE != nil {
		matched := matchSources(sources, sourceMatchRE)
		fmt.Fprintf(statusW, "--source-match %q: %d source(s)\n", opts.SourceMatch, len(matched))
		for _, source := range matched {
			sourceSet[source.ID] = true
		}
	}
	labelSet := make(map[string]bool)
	for _, label := range matchLabels(labels, flagLabelIDs, labelMatchRE) {
		fmt.Fprintf(statusW, "label %q (%s): %d source(s)\n", label.Name, label.LabelID, len(label.SourceIDs))
		for _, id := range label.SourceIDs {
			labelSet[id] = true
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
				labelSet[source.ID] = true
			}
		}
	}
	excluded := make(map[string]bool)
	if sourceExcludeRE != nil {
		matches := matchSources(sources, sourceExcludeRE)
		fmt.Fprintf(statusW, "--source-exclude %q: %d source(s)\n", opts.SourceExclude, len(matches))
		for _, source := range matches {
			excluded[source.ID] = true
		}
	}
	for _, label := range matchLabels(labels, flagLabelExcludeIDs, labelExcludeRE) {
		for _, id := range label.SourceIDs {
			excluded[id] = true
		}
	}
	result := selection{Explicit: true, known: knownSources, excluded: excluded}
	seen := make(map[string]bool)
	for _, source := range sources {
		id := source.ID
		include := (!sourceActive || sourceSet[id]) && (!labelActive || labelSet[id])
		if opts.Mode == "union" && (sourceActive || labelActive) {
			include = sourceSet[id] || labelSet[id]
		}
		if include && knownSources[id] && !excluded[id] && !seen[id] {
			result.IDs = append(result.IDs, id)
			seen[id] = true
		}
	}
	if len(result.IDs) == 0 {
		return selection{}, fmt.Errorf("selectors resolved to empty set after exclusions (source IDs %q, source match %q, label IDs %q, label match %q, label-none %t, source exclude %q, label exclude %q, label exclude IDs %q)", opts.SourceIDs, opts.SourceMatch, opts.LabelIDs, opts.LabelMatch, opts.LabelNone, opts.SourceExclude, opts.LabelExclude, opts.LabelExcludeIDs)
	}
	return result, nil
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
