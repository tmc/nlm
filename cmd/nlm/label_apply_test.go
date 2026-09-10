package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/tmc/nlm/notebooklm"
)

func testSnapshot() *labelSnapshot {
	labels := []notebooklm.Label{
		{LabelID: "lbl-misc", Name: "Miscellaneous", SourceIDs: []string{"src-a", "src-b"}},
		{LabelID: "lbl-agent", Name: "coding agent session", SourceIDs: []string{"src-b"}},
	}
	snapshot := &labelSnapshot{
		labels:         labels,
		titles:         map[string]string{"src-a": "alpha.md", "src-b": "beta.md", "src-c": "gamma.md"},
		labelsBySource: map[string][]string{},
		nameByLabel:    map[string]string{},
	}
	for _, label := range labels {
		snapshot.nameByLabel[label.LabelID] = label.Name
		for _, id := range label.SourceIDs {
			snapshot.labelsBySource[id] = append(snapshot.labelsBySource[id], label.LabelID)
		}
	}
	return snapshot
}

func TestPlanLabelApply(t *testing.T) {
	tests := []struct {
		name       string
		target     string
		sources    []string
		opts       labelApplyOptions
		wantAttach map[string][]string
		wantDetach map[string][]string
		wantAfter  map[string][]string
	}{
		{
			name:       "attach adds only where missing",
			target:     "lbl-agent",
			sources:    []string{"src-a", "src-b"},
			wantAttach: map[string][]string{"src-a": {"lbl-agent"}},
			wantDetach: map[string][]string{},
			wantAfter:  map[string][]string{"src-a": {"lbl-agent", "lbl-misc"}, "src-b": {"lbl-agent", "lbl-misc"}},
		},
		{
			name:       "exclusive strips the other labels",
			target:     "lbl-agent",
			sources:    []string{"src-a", "src-b"},
			opts:       labelApplyOptions{Exclusive: true},
			wantAttach: map[string][]string{"src-a": {"lbl-agent"}},
			wantDetach: map[string][]string{"src-a": {"lbl-misc"}, "src-b": {"lbl-misc"}},
			wantAfter:  map[string][]string{"src-a": {"lbl-agent"}, "src-b": {"lbl-agent"}},
		},
		{
			name:       "detach only where held",
			target:     "lbl-agent",
			sources:    []string{"src-a", "src-b"},
			opts:       labelApplyOptions{Detach: true},
			wantAttach: map[string][]string{},
			wantDetach: map[string][]string{"src-b": {"lbl-agent"}},
			wantAfter:  map[string][]string{"src-a": {"lbl-misc"}, "src-b": {"lbl-misc"}},
		},
		{
			name:       "attaching an unlabeled source",
			target:     "lbl-agent",
			sources:    []string{"src-c"},
			opts:       labelApplyOptions{Exclusive: true},
			wantAttach: map[string][]string{"src-c": {"lbl-agent"}},
			wantDetach: map[string][]string{},
			wantAfter:  map[string][]string{"src-c": {"lbl-agent"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			changes := planLabelApply(testSnapshot(), tt.target, tt.sources, tt.opts)
			if len(changes) != len(tt.sources) {
				t.Fatalf("got %d changes, want %d", len(changes), len(tt.sources))
			}
			for _, change := range changes {
				if want := tt.wantAttach[change.SourceID]; !equalStrings(change.attach, want) {
					t.Errorf("%s attach = %v, want %v", change.SourceID, change.attach, want)
				}
				if want := tt.wantDetach[change.SourceID]; !equalStrings(change.detach, want) {
					t.Errorf("%s detach = %v, want %v", change.SourceID, change.detach, want)
				}
				if want := tt.wantAfter[change.SourceID]; !equalStrings(change.After, want) {
					t.Errorf("%s after = %v, want %v", change.SourceID, change.After, want)
				}
				wantChanged := len(tt.wantAttach[change.SourceID])+len(tt.wantDetach[change.SourceID]) > 0
				if change.Changed != wantChanged {
					t.Errorf("%s changed = %v, want %v", change.SourceID, change.Changed, wantChanged)
				}
			}
		})
	}
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestResolveLabelApplySources(t *testing.T) {
	tests := []struct {
		name    string
		opts    labelApplyOptions
		want    []string
		wantErr string
	}{
		{
			name: "ids and titles and comma lists",
			opts: labelApplyOptions{Sources: []string{"src-a", "beta.md", "src-c"}},
			want: []string{"src-a", "src-b", "src-c"},
		},
		{
			name: "duplicates collapse",
			opts: labelApplyOptions{Sources: []string{"src-a", "alpha.md"}},
			want: []string{"src-a"},
		},
		{
			name: "label selector expands to members",
			opts: labelApplyOptions{Selectors: selectorOptions{LabelMatch: "^Miscellaneous$"}},
			want: []string{"src-a", "src-b"},
		},
		{
			name: "positional and selector union",
			opts: labelApplyOptions{Sources: []string{"src-c"}, Selectors: selectorOptions{LabelMatch: "^coding"}},
			want: []string{"src-c", "src-b"},
		},
		{
			name:    "unknown title",
			opts:    labelApplyOptions{Sources: []string{"nope.md"}},
			wantErr: "no source titled",
		},
		{
			name:    "nothing selected",
			opts:    labelApplyOptions{},
			wantErr: "no sources selected",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveLabelApplySources(testSnapshot(), tt.opts)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !equalStrings(got, tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRenderLabelChanges(t *testing.T) {
	snapshot := testSnapshot()
	changes := planLabelApply(snapshot, "lbl-agent", []string{"src-a", "src-b"}, labelApplyOptions{Exclusive: true})

	var out, status bytes.Buffer
	if err := renderLabelChanges(&out, &status, snapshot, changes, false, false); err != nil {
		t.Fatalf("renderLabelChanges: %v", err)
	}
	for _, want := range []string{
		"SOURCE\tBEFORE\tAFTER",
		"alpha.md\tMiscellaneous\tcoding agent session",
		"beta.md\tcoding agent session|Miscellaneous\tcoding agent session",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("stdout missing %q\ngot:\n%s", want, out.String())
		}
	}
	if !strings.Contains(status.String(), "Updated 2 of 2 source(s)") {
		t.Errorf("status = %q", status.String())
	}

	out.Reset()
	status.Reset()
	if err := renderLabelChanges(&out, &status, snapshot, changes, false, true); err != nil {
		t.Fatalf("renderLabelChanges dry run: %v", err)
	}
	if !strings.Contains(status.String(), "Would update 2 of 2 source(s)") {
		t.Errorf("dry-run status = %q", status.String())
	}

	out.Reset()
	status.Reset()
	if err := renderLabelChanges(&out, &status, snapshot, changes, true, false); err != nil {
		t.Fatalf("renderLabelChanges json: %v", err)
	}
	var first labelChange
	line, _, _ := strings.Cut(out.String(), "\n")
	if err := json.Unmarshal([]byte(line), &first); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	if first.SourceID != "src-a" || first.Title != "alpha.md" || !first.Changed {
		t.Errorf("json record = %+v", first)
	}
	if !equalStrings(first.Before, []string{"lbl-misc"}) || !equalStrings(first.After, []string{"lbl-agent"}) {
		t.Errorf("json membership = %+v", first)
	}
}

func TestLabelSnapshotResolve(t *testing.T) {
	snapshot := testSnapshot()
	snapshot.labels = append(snapshot.labels, notebooklm.Label{LabelID: "lbl-dup", Name: "miscellaneous"})
	snapshot.nameByLabel["lbl-dup"] = "miscellaneous"

	if _, err := snapshot.resolveLabel("Miscellaneous"); err == nil ||
		!strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("ambiguous label error = %v", err)
	}
	if got, err := snapshot.resolveLabel("lbl-agent"); err != nil || got != "lbl-agent" {
		t.Errorf("resolveLabel(id) = %q, %v", got, err)
	}
	if _, err := snapshot.resolveLabel("11111111-1111-1111-1111-111111111111"); err == nil ||
		!strings.Contains(err.Error(), "no label") {
		t.Errorf("unknown label UUID error = %v", err)
	}
	if _, err := snapshot.resolveSource("11111111-1111-1111-1111-111111111111"); err == nil ||
		!strings.Contains(err.Error(), "no source") {
		t.Errorf("unknown source UUID error = %v", err)
	}
}
