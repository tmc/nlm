package nlmsync

import (
	"context"
	"fmt"
	"sort"
)

// labelPlan is built before mutations. Maps used by upload workers are then
// immutable; each part receives its own labels or a specified donor union.
type labelPlan struct {
	base           string
	seed           []string
	byTitle        map[string][]string
	bySource       map[string][]string
	common         []string
	renames        []Source
	recoveries     []Source
	canonical      map[string]Source
	familyCollapse bool
}

func unionLabels(sets ...[]string) []string {
	seen := make(map[string]bool)
	for _, set := range sets {
		for _, id := range set {
			if id != "" {
				seen[id] = true
			}
		}
	}
	var ids []string
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func missingLabels(want, have []string) []string {
	seen := make(map[string]bool)
	for _, id := range have {
		seen[id] = true
	}
	var missing []string
	for _, id := range want {
		if !seen[id] {
			missing = append(missing, id)
		}
	}
	return missing
}

func planLabels(ctx context.Context, c Client, notebookID, base string, names []string, sources []Source, opts Options) (*labelPlan, error) {
	if opts.NoLabels && len(opts.Labels) > 0 {
		return nil, fmt.Errorf("cannot seed labels with NoLabels set")
	}
	p := &labelPlan{base: base, seed: unionLabels(opts.Labels), byTitle: make(map[string][]string), bySource: make(map[string][]string), canonical: make(map[string]Source)}
	lp, capable := c.(LabelPreserver)
	if !opts.NoLabels && !capable {
		return nil, fmt.Errorf("cannot plan labels: client lacks LabelPreserver; set Options.NoLabels to explicitly omit label preservation")
	}
	chunks := make(map[string]bool)
	for _, source := range sources {
		identity, old, owned := parsePart(source.Title, base)
		if !owned {
			continue
		}
		name := identity.title(base)
		chunks[identity.chunk] = true
		var ids []string
		if !opts.NoLabels {
			var err error
			ids, err = lp.LabelsForSource(ctx, notebookID, source.ID)
			if err != nil {
				return nil, fmt.Errorf("read labels for %q: %w", source.Title, err)
			}
		}
		p.bySource[source.ID] = unionLabels(ids)
		if old {
			p.recoveries = append(p.recoveries, source)
			continue
		}
		p.byTitle[name] = p.bySource[source.ID]
		if name != source.Title {
			p.renames = append(p.renames, Source{ID: source.ID, Title: name})
		}
		source.Title = name
		p.canonical[name] = source
	}
	// Recovery donors take precedence even over an empty canonical leaf.
	for _, old := range p.recoveries {
		identity, _, _ := parsePart(old.Title, base)
		name := identity.title(base)
		p.byTitle[name] = unionLabels(p.byTitle[name], p.bySource[old.ID])
		if _, ok := p.canonical[name]; !ok {
			old.Title = name
			p.canonical[name] = old
			p.renames = append(p.renames, old)
		}
	}
	p.common = commonLabels(p.byTitle)
	if len(names) == 1 {
		for chunk := range chunks {
			if chunk != "1" {
				p.familyCollapse = true
			}
		}
		return p, nil
	}
	expected := make(map[string]bool)
	for _, name := range names {
		id, _, _ := parsePart(name, base)
		expected[id.chunk] = true
	}
	// A chunk that keeps its slot keeps its labels, so a family that only
	// gains chunks needs no donor evidence: nothing is deleted and nothing
	// has to be attributed. Only a labeled chunk that disappears would lose
	// its assignments to a donor the manifest cannot identify.
	var lost []string
	for _, name := range sortedPartNames(p.byTitle) {
		id, _, _ := parsePart(name, base)
		if len(p.byTitle[name]) > 0 && !expected[id.chunk] {
			lost = append(lost, name)
		}
	}
	if len(lost) > 0 {
		return nil, fmt.Errorf("ambiguous labeled rechunk of %q (%v): detach labels or sync into a fresh family name", base, lost)
	}
	return p, nil
}

// commonLabels returns the labels every existing part of a family carries.
// A part minted by a later sync joins a classification the family already
// agrees on; it never inherits a label only some siblings hold.
func commonLabels(byTitle map[string][]string) []string {
	names := sortedPartNames(byTitle)
	if len(names) == 0 {
		return nil
	}
	common := byTitle[names[0]]
	for _, name := range names[1:] {
		have := make(map[string]bool, len(byTitle[name]))
		for _, id := range byTitle[name] {
			have[id] = true
		}
		var keep []string
		for _, id := range common {
			if have[id] {
				keep = append(keep, id)
			}
		}
		common = keep
	}
	return unionLabels(common)
}

func sortedPartNames(parts map[string][]string) []string {
	var names []string
	for name := range parts {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// labels returns the label IDs a part should carry: its own inherited set
// (or, when collapsing a family, the union across its descendants) plus the
// seed from Options.Labels. Seeding at this single point covers every attach
// site, so parts minted by auto-splitting inherit the seed too.
func (p *labelPlan) labels(name string, inherited []string, collapse bool) []string {
	if collapse {
		sets := [][]string{inherited, p.seed, p.common}
		for _, title := range sortedPartNames(p.byTitle) {
			if title == name || partDescendant(p.base, title, name) || (name == p.base && p.familyCollapse) {
				sets = append(sets, p.byTitle[title])
			}
		}
		return unionLabels(sets...)
	}
	if ids, ok := p.byTitle[name]; ok {
		if len(p.seed) == 0 {
			return ids
		}
		return unionLabels(ids, p.seed)
	}
	if len(p.seed) == 0 && len(p.common) == 0 {
		return inherited
	}
	return unionLabels(inherited, p.seed, p.common)
}

func emitLabelPlan(out *outputWriter, name, sourceID string, ids []string) {
	for _, id := range ids {
		out.emit(event{Action: "label", Name: name, SourceID: sourceID, LabelID: id, DryRun: true})
	}
}

// recover transfers labels before deleting a stranded replacement donor. All
// reads used to authorize the plan have already succeeded in planLabels.
func (p *labelPlan) recover(ctx context.Context, c Client, notebookID string, sources []Source, opts Options, sc *sourceCache, out *outputWriter) ([]Source, error) {
	for _, source := range p.renames {
		if !opts.DryRun {
			if err := c.RenameSource(ctx, source.ID, source.Title); err != nil {
				return nil, fmt.Errorf("restore canonical title %q: %w", source.Title, err)
			}
		}
		out.emit(event{Action: "rename", Name: source.Title, SourceID: source.ID, Reason: "canonical identity", DryRun: opts.DryRun})
	}
	removed := make(map[string]bool)
	for _, old := range p.recoveries {
		identity, _, _ := parsePart(old.Title, p.base)
		name := identity.title(p.base)
		target := p.canonical[name]
		if target.ID == old.ID {
			continue
		}
		missing := missingLabels(p.byTitle[name], p.bySource[target.ID])
		if opts.DryRun {
			emitLabelPlan(out, name, target.ID, missing)
		} else if !opts.NoLabels {
			lp := c.(LabelPreserver)
			for _, id := range missing {
				if err := lp.AttachLabelSource(ctx, notebookID, id, target.ID); err != nil {
					return nil, fmt.Errorf("recover label %s for %q: %w", id, name, err)
				}
			}
			have, err := lp.LabelsForSource(ctx, notebookID, target.ID)
			if err != nil {
				return nil, fmt.Errorf("verify recovered labels for %q: %w", name, err)
			}
			if len(missingLabels(p.byTitle[name], have)) > 0 {
				return nil, fmt.Errorf("verify recovered labels for %q: assignments missing", name)
			}
		}
		if !opts.DryRun {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if err := c.DeleteSources(ctx, notebookID, []string{old.ID}); err != nil {
				return nil, fmt.Errorf("delete recovery donor %q: %w", old.Title, err)
			}
		}
		out.emit(event{Action: "delete", Name: old.Title, OldID: old.ID, Reason: "labels recovered", DryRun: opts.DryRun})
		removed[old.ID] = true
		p.bySource[target.ID] = p.byTitle[name]
	}
	var result []Source
	for _, source := range sources {
		if removed[source.ID] {
			continue
		}
		if identity, _, ok := parsePart(source.Title, p.base); ok {
			source.Title = identity.title(p.base)
		}
		result = append(result, source)
	}
	if !opts.DryRun {
		_ = sc.save(notebookID, result)
	}
	return result, nil
}
