package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/tmc/nlm/notebooklm"
)

var uuidRE = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// labelApplyParallel bounds the concurrent le8sX calls a single attach or
// detach run makes. The wire takes one source per call, so a 163-source
// relabel is 163 requests; the web UI fires them concurrently and this keeps
// that shape without opening an unbounded number of connections.
const labelApplyParallel = 8

// labelSnapshot is the notebook state an attach or detach run resolves
// against: source titles and current label membership, read once. Membership
// after the run is computed from the plan rather than re-read, because
// GetLabels is eventually consistent and a read straight after a write can
// still report the old set.
type labelSnapshot struct {
	labels []notebooklm.Label
	titles map[string]string
	// labelsBySource maps a source ID to the label IDs holding it.
	labelsBySource map[string][]string
	nameByLabel    map[string]string
}

func readLabelSnapshot(ctx context.Context, client *notebooklm.Client, notebookID string) (*labelSnapshot, error) {
	project, err := client.GetProject(ctx, notebookID)
	if err != nil {
		return nil, fmt.Errorf("list sources: %w", err)
	}
	labels, err := client.GetLabels(ctx, notebookID)
	if err != nil {
		return nil, fmt.Errorf("list labels: %w", err)
	}
	snapshot := &labelSnapshot{
		labels:         labels,
		titles:         make(map[string]string, len(project.Sources)),
		labelsBySource: make(map[string][]string),
		nameByLabel:    make(map[string]string, len(labels)),
	}
	for _, source := range project.Sources {
		if id := source.SourceId.GetSourceId(); id != "" {
			snapshot.titles[id] = strings.TrimSpace(source.Title)
		}
	}
	for _, label := range labels {
		snapshot.nameByLabel[label.LabelID] = label.Name
		for _, id := range label.SourceIDs {
			snapshot.labelsBySource[id] = append(snapshot.labelsBySource[id], label.LabelID)
		}
	}
	return snapshot, nil
}

// sourceSummaries projects the snapshot for selector resolution.
func (s *labelSnapshot) sourceSummaries() []sourceSummary {
	out := make([]sourceSummary, 0, len(s.titles))
	for id, title := range s.titles {
		out = append(out, sourceSummary{ID: id, Title: title})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// resolveLabel maps a label ID or name to a label ID. An argument that is
// already a known ID wins; otherwise names are matched case-insensitively and
// must be unambiguous.
func (s *labelSnapshot) resolveLabel(arg string) (string, error) {
	return findLabel(s.labels, arg)
}

// resolveSource maps a source ID or title to a source ID, preferring a known
// ID over a title match.
func (s *labelSnapshot) resolveSource(arg string) (string, error) {
	if _, ok := s.titles[arg]; ok {
		return arg, nil
	}
	want := strings.ToLower(arg)
	var matches []string
	for id, title := range s.titles {
		if strings.ToLower(title) == want {
			matches = append(matches, id)
		}
	}
	sort.Strings(matches)
	switch {
	case len(matches) == 1:
		return matches[0], nil
	case len(matches) > 1:
		return "", fmt.Errorf("source title %q is ambiguous (%d matches); pass the source ID instead", arg, len(matches))
	case uuidRE.MatchString(arg):
		return "", fmt.Errorf("no source %s in notebook (use 'nlm source list' to see options)", arg)
	default:
		return "", fmt.Errorf("no source titled %q in notebook (use 'nlm source list' to see options)", arg)
	}
}

// labelChange is one source's before and after membership.
type labelChange struct {
	SourceID string   `json:"source_id"`
	Title    string   `json:"title"`
	Before   []string `json:"labels_before"`
	After    []string `json:"labels_after"`
	Changed  bool     `json:"changed"`

	// attach and detach are the label IDs to add and remove for this source.
	attach []string
	detach []string
}

type labelApplyOptions struct {
	NotebookID string
	Label      string
	Sources    []string
	Selectors  selectorOptions
	Detach     bool
	Exclusive  bool
	Create     bool
	DryRun     bool
	JSON       bool
}

// resolveLabelApplySources gathers the target source IDs from the positional
// arguments and the selector flags, preserving notebook order and dropping
// duplicates.
func resolveLabelApplySources(snapshot *labelSnapshot, opts labelApplyOptions) ([]string, error) {
	seen := make(map[string]bool)
	var ids []string
	add := func(id string) {
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	for _, arg := range opts.Sources {
		expanded, err := resolveIDList(arg)
		if err != nil {
			return nil, err
		}
		for _, item := range expanded {
			id, err := snapshot.resolveSource(item)
			if err != nil {
				return nil, err
			}
			add(id)
		}
	}
	if !opts.Selectors.empty() {
		flagIDs, err := resolveIDList(opts.Selectors.SourceIDs)
		if err != nil {
			return nil, fmt.Errorf("--source-ids: %w", err)
		}
		flagLabelIDs, err := resolveIDList(opts.Selectors.LabelIDs)
		if err != nil {
			return nil, fmt.Errorf("--label-ids: %w", err)
		}
		flagExcludeIDs, err := resolveIDList(opts.Selectors.LabelExcludeIDs)
		if err != nil {
			return nil, fmt.Errorf("--label-exclude-ids: %w", err)
		}
		selected, err := resolveSelectorIDs(opts.Selectors, flagIDs, flagLabelIDs, flagExcludeIDs,
			snapshot.sourceSummaries(), snapshot.labels, os.Stderr)
		if err != nil {
			return nil, err
		}
		for _, id := range selected.IDs {
			add(id)
		}
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("no sources selected; pass source IDs or names, '-' for a newline-delimited list on stdin, or selector flags")
	}
	return ids, nil
}

// planLabelApply computes the per-source work without making any calls.
func planLabelApply(snapshot *labelSnapshot, targetLabel string, sourceIDs []string, opts labelApplyOptions) []labelChange {
	changes := make([]labelChange, 0, len(sourceIDs))
	for _, id := range sourceIDs {
		before := append([]string(nil), snapshot.labelsBySource[id]...)
		sort.Strings(before)
		holds := false
		for _, labelID := range before {
			if labelID == targetLabel {
				holds = true
			}
		}
		change := labelChange{SourceID: id, Title: snapshot.titles[id], Before: before}
		switch {
		case opts.Detach:
			if holds {
				change.detach = []string{targetLabel}
			}
		default:
			if !holds {
				change.attach = []string{targetLabel}
			}
			if opts.Exclusive {
				for _, labelID := range before {
					if labelID != targetLabel {
						change.detach = append(change.detach, labelID)
					}
				}
			}
		}
		after := make([]string, 0, len(before)+1)
		dropped := make(map[string]bool, len(change.detach))
		for _, labelID := range change.detach {
			dropped[labelID] = true
		}
		for _, labelID := range before {
			if !dropped[labelID] {
				after = append(after, labelID)
			}
		}
		after = append(after, change.attach...)
		sort.Strings(after)
		change.After = after
		change.Changed = len(change.attach)+len(change.detach) > 0
		changes = append(changes, change)
	}
	return changes
}

// applyLabelChanges issues the mutations. Each attach and detach is its own
// request: the server honors only the first source ID in a call, and drops
// the add when a call carries both an add and a remove.
func applyLabelChanges(ctx context.Context, client *notebooklm.Client, notebookID string, changes []labelChange) error {
	type job struct {
		sourceID string
		labelID  string
		detach   bool
	}
	var jobs []job
	for _, change := range changes {
		for _, labelID := range change.detach {
			jobs = append(jobs, job{sourceID: change.SourceID, labelID: labelID, detach: true})
		}
		for _, labelID := range change.attach {
			jobs = append(jobs, job{sourceID: change.SourceID, labelID: labelID})
		}
	}
	if len(jobs) == 0 {
		return nil
	}
	workers := labelApplyParallel
	if len(jobs) < workers {
		workers = len(jobs)
	}
	var (
		mu    sync.Mutex
		errs  []error
		wg    sync.WaitGroup
		queue = make(chan job)
	)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range queue {
				var err error
				if j.detach {
					err = client.DetachLabelSource(ctx, notebookID, j.labelID, j.sourceID)
				} else {
					err = client.AttachLabelSource(ctx, notebookID, j.labelID, j.sourceID)
				}
				if err != nil {
					verb := "attach"
					if j.detach {
						verb = "detach"
					}
					mu.Lock()
					errs = append(errs, fmt.Errorf("%s %s %s: %w", verb, j.sourceID, j.labelID, err))
					mu.Unlock()
				}
			}
		}()
	}
	for _, j := range jobs {
		queue <- j
	}
	close(queue)
	wg.Wait()
	if len(errs) > 0 {
		return fmt.Errorf("%d of %d label mutation(s) failed: %w", len(errs), len(jobs), errs[0])
	}
	return nil
}

// renderLabelChanges reports what each source's labels were and now are.
// Names come from the snapshot, so a label created by this run is named too.
func renderLabelChanges(out, status io.Writer, snapshot *labelSnapshot, changes []labelChange, jsonOutput, dryRun bool) error {
	names := func(ids []string) string {
		if len(ids) == 0 {
			return "(none)"
		}
		parts := make([]string, 0, len(ids))
		for _, id := range ids {
			if name := snapshot.nameByLabel[id]; name != "" {
				parts = append(parts, name)
			} else {
				parts = append(parts, id)
			}
		}
		return strings.Join(parts, "|")
	}
	if jsonOutput {
		enc := json.NewEncoder(out)
		for _, change := range changes {
			if err := enc.Encode(change); err != nil {
				return err
			}
		}
		return nil
	}
	w := out
	flush := func() error { return nil }
	if f, ok := out.(*os.File); ok {
		w, flush = newListWriter(f)
	}
	fmt.Fprintln(w, "SOURCE\tBEFORE\tAFTER")
	changed := 0
	for _, change := range changes {
		title := change.Title
		if title == "" {
			title = change.SourceID
		}
		if change.Changed {
			changed++
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", title, names(change.Before), names(change.After))
	}
	if err := flush(); err != nil {
		return err
	}
	verb := "Updated"
	if dryRun {
		verb = "Would update"
	}
	fmt.Fprintf(status, "%s %d of %d source(s)\n", verb, changed, len(changes))
	return nil
}

func runLabelApply(ctx context.Context, client *notebooklm.Client, opts labelApplyOptions) error {
	if err := opts.Selectors.validateStdin(hasStdinSourceArg(opts.Sources)); err != nil {
		return err
	}
	snapshot, err := readLabelSnapshot(ctx, client, opts.NotebookID)
	if err != nil {
		return err
	}
	targetLabel, err := snapshot.resolveLabel(opts.Label)
	if err != nil {
		if !opts.Create || opts.Detach || !isLabelNotFound(err) || uuidRE.MatchString(opts.Label) {
			return err
		}
		if opts.DryRun {
			return fmt.Errorf("%w (--create would create it)", err)
		}
		if _, cerr := client.CreateLabel(ctx, opts.NotebookID, opts.Label, ""); cerr != nil {
			return cerr
		}
		if snapshot, err = readLabelSnapshot(ctx, client, opts.NotebookID); err != nil {
			return err
		}
		if targetLabel, err = snapshot.resolveLabel(opts.Label); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "Created label %q (%s)\n", opts.Label, targetLabel)
	}
	sourceIDs, err := resolveLabelApplySources(snapshot, opts)
	if err != nil {
		return err
	}
	changes := planLabelApply(snapshot, targetLabel, sourceIDs, opts)
	if !opts.DryRun {
		if err := applyLabelChanges(ctx, client, opts.NotebookID, changes); err != nil {
			return err
		}
	}
	return renderLabelChanges(os.Stdout, os.Stderr, snapshot, changes, opts.JSON, opts.DryRun)
}

func hasStdinSourceArg(args []string) bool {
	for _, arg := range args {
		if arg == "-" {
			return true
		}
	}
	return false
}

// sourceTitles reads the notebook's source titles keyed by source ID.
func sourceTitles(ctx context.Context, client *notebooklm.Client, notebookID string) (map[string]string, error) {
	project, err := client.GetProject(ctx, notebookID)
	if err != nil {
		return nil, fmt.Errorf("list sources: %w", err)
	}
	titles := make(map[string]string, len(project.Sources))
	for _, source := range project.Sources {
		if id := source.SourceId.GetSourceId(); id != "" {
			titles[id] = strings.TrimSpace(source.Title)
		}
	}
	return titles, nil
}
