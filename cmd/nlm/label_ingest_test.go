package main

import (
	"context"
	"strings"
	"testing"

	"github.com/tmc/nlm/notebooklm"
)

type fakeLabelEnsurer struct {
	labels  []notebooklm.Label
	created []string
	err     error
}

func (f *fakeLabelEnsurer) GetLabels(context.Context, string) ([]notebooklm.Label, error) {
	return f.labels, f.err
}

func (f *fakeLabelEnsurer) CreateLabel(_ context.Context, _, name, _ string) ([]notebooklm.Label, error) {
	f.created = append(f.created, name)
	f.labels = append(f.labels, notebooklm.Label{LabelID: "new-" + name, Name: name})
	return f.labels, nil
}

var ingestLabels = []notebooklm.Label{
	{LabelID: "11111111-1111-4111-8111-111111111111", Name: "Miscellaneous"},
	{LabelID: "22222222-2222-4222-8222-222222222222", Name: "triage"},
	{LabelID: "33333333-3333-4333-8333-333333333333", Name: "triage"},
}

func TestFindLabel(t *testing.T) {
	for _, tt := range []struct {
		name, arg, want, wantErr string
		notFound                 bool
	}{
		{name: "by id", arg: "11111111-1111-4111-8111-111111111111", want: "11111111-1111-4111-8111-111111111111"},
		{name: "by name", arg: "Miscellaneous", want: "11111111-1111-4111-8111-111111111111"},
		{name: "case insensitive", arg: "miscellaneous", want: "11111111-1111-4111-8111-111111111111"},
		{name: "ambiguous", arg: "triage", wantErr: "ambiguous"},
		{name: "unknown name", arg: "nope", wantErr: "no label named", notFound: true},
		{name: "unknown id", arg: "44444444-4444-4444-8444-444444444444", wantErr: "no label 4444", notFound: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := findLabel(ingestLabels, tt.arg)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("findLabel(%q) error = %v, want %q", tt.arg, err, tt.wantErr)
				}
				if isLabelNotFound(err) != tt.notFound {
					t.Fatalf("findLabel(%q) notFound = %v, want %v", tt.arg, isLabelNotFound(err), tt.notFound)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("findLabel(%q) = %q, %v; want %q", tt.arg, got, err, tt.want)
			}
		})
	}
}

func TestResolveIngestLabel(t *testing.T) {
	ctx := context.Background()

	t.Run("existing", func(t *testing.T) {
		c := &fakeLabelEnsurer{labels: ingestLabels}
		got, err := resolveIngestLabel(ctx, c, "nb", "Miscellaneous", false)
		if err != nil || got != "11111111-1111-4111-8111-111111111111" {
			t.Fatalf("= %q, %v", got, err)
		}
		if len(c.created) != 0 {
			t.Fatalf("created %v", c.created)
		}
	})

	t.Run("missing without create", func(t *testing.T) {
		c := &fakeLabelEnsurer{labels: ingestLabels}
		_, err := resolveIngestLabel(ctx, c, "nb", "fresh", false)
		if err == nil || !strings.Contains(err.Error(), "--create-label") {
			t.Fatalf("err = %v, want a --create-label hint", err)
		}
	})

	t.Run("missing with create", func(t *testing.T) {
		c := &fakeLabelEnsurer{labels: ingestLabels}
		got, err := resolveIngestLabel(ctx, c, "nb", "fresh", true)
		if err != nil || got != "new-fresh" {
			t.Fatalf("= %q, %v", got, err)
		}
		if len(c.created) != 1 || c.created[0] != "fresh" {
			t.Fatalf("created %v", c.created)
		}
	})

	// Ambiguity must not be resolved by minting a third label of that name.
	t.Run("ambiguous with create", func(t *testing.T) {
		c := &fakeLabelEnsurer{labels: ingestLabels}
		_, err := resolveIngestLabel(ctx, c, "nb", "triage", true)
		if err == nil || !strings.Contains(err.Error(), "ambiguous") {
			t.Fatalf("err = %v, want ambiguous", err)
		}
		if len(c.created) != 0 {
			t.Fatalf("created %v", c.created)
		}
	})

	// A UUID is an ID that does not exist, not a name to create.
	t.Run("unknown id with create", func(t *testing.T) {
		c := &fakeLabelEnsurer{labels: ingestLabels}
		_, err := resolveIngestLabel(ctx, c, "nb", "44444444-4444-4444-8444-444444444444", true)
		if err == nil || len(c.created) != 0 {
			t.Fatalf("err = %v, created %v", err, c.created)
		}
	})
}
