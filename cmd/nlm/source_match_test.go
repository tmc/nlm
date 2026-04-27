package main

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/tmc/nlm/internal/notebooklm/api"
)

func TestResolveSelectorIDs(t *testing.T) {
	srcs := []sourceSummary{
		{ID: "src-spec-1", Title: "spec/architecture"},
		{ID: "src-spec-2", Title: "spec/api"},
		{ID: "src-impl-1", Title: "impl/server"},
		{ID: "src-impl-2", Title: "impl/client"},
		{ID: "src-draft-1", Title: "spec/draft-notes"},
	}
	labels := []api.Label{
		{LabelID: "lbl-test", Name: "Testing", SourceIDs: []string{"src-impl-1", "src-impl-2"}},
		{LabelID: "lbl-rpc", Name: "RPC and Networking", SourceIDs: []string{"src-spec-2", "src-impl-1"}},
		{LabelID: "lbl-draft", Name: "Draft", SourceIDs: []string{"src-draft-1"}},
	}

	tests := []struct {
		name      string
		opts      selectorOptions
		flagIDs   []string
		labelIDs  []string
		want      []string
		wantErr   string
		wantStatusContains []string
	}{
		{
			name: "no selectors returns nil",
			opts: selectorOptions{},
			want: nil,
		},
		{
			name:    "source-ids only",
			opts:    selectorOptions{SourceIDs: "src-spec-1,src-impl-1"},
			flagIDs: []string{"src-spec-1", "src-impl-1"},
			want:    []string{"src-spec-1", "src-impl-1"},
		},
		{
			name: "source-match only",
			opts: selectorOptions{SourceMatch: "^spec/"},
			want: []string{"src-spec-1", "src-spec-2", "src-draft-1"},
			wantStatusContains: []string{"--source-match", "3 source(s)"},
		},
		{
			name: "source-match no hits errors and lists",
			opts: selectorOptions{SourceMatch: "^never/"},
			wantErr: "--source-match matched no sources",
			wantStatusContains: []string{"matched no sources", "spec/architecture"},
		},
		{
			name: "source-exclude alone is all-minus",
			opts: selectorOptions{SourceExclude: "draft"},
			want: []string{"src-spec-1", "src-spec-2", "src-impl-1", "src-impl-2"},
			wantStatusContains: []string{"--source-exclude"},
		},
		{
			name: "source-match plus source-exclude subtracts",
			opts: selectorOptions{SourceMatch: "^spec/", SourceExclude: "draft"},
			want: []string{"src-spec-1", "src-spec-2"},
		},
		{
			name: "exclude wins over include",
			opts: selectorOptions{SourceIDs: "src-draft-1,src-spec-1", SourceExclude: "draft"},
			flagIDs: []string{"src-draft-1", "src-spec-1"},
			want: []string{"src-spec-1"},
		},
		{
			name: "label-match include OR semantics",
			opts: selectorOptions{LabelMatch: "^Testing$"},
			want: []string{"src-impl-1", "src-impl-2"},
			wantStatusContains: []string{"label \"Testing\""},
		},
		{
			name: "label-ids include unioned with label-match",
			opts: selectorOptions{LabelMatch: "^Testing$", LabelIDs: "lbl-rpc"},
			labelIDs: []string{"lbl-rpc"},
			// Order follows the labels slice (Testing first, then RPC); src-impl-1
			// appears once via the dedup map even though both labels include it.
			want: []string{"src-impl-1", "src-impl-2", "src-spec-2"},
		},
		{
			name: "label-exclude removes tagged sources",
			opts: selectorOptions{SourceMatch: "^impl/", LabelExclude: "^Testing$"},
			want: nil,
			wantErr: "selectors resolved to empty set after exclusions",
		},
		{
			name: "label-exclude alone subtracts from all",
			opts: selectorOptions{LabelExclude: "^Draft$"},
			want: []string{"src-spec-1", "src-spec-2", "src-impl-1", "src-impl-2"},
		},
		{
			name: "label include with no match errors",
			opts: selectorOptions{LabelMatch: "^Nonexistent$"},
			wantErr: "label selectors matched no labels",
			wantStatusContains: []string{"matched no labels", "Testing", "Draft"},
		},
		{
			name:    "invalid source-match regex",
			opts:    selectorOptions{SourceMatch: "[bad"},
			wantErr: "--source-match: invalid regex",
		},
		{
			name:    "invalid label-exclude regex",
			opts:    selectorOptions{LabelExclude: "[bad"},
			wantErr: "--label-exclude: invalid regex",
		},
		{
			name: "source-ids unioned with source-match",
			opts: selectorOptions{SourceIDs: "src-impl-2", SourceMatch: "^spec/"},
			flagIDs: []string{"src-impl-2"},
			want: []string{"src-impl-2", "src-spec-1", "src-spec-2", "src-draft-1"},
		},
		{
			name: "exclusions only with empty include list still returns all-minus",
			opts: selectorOptions{LabelExclude: "^Draft$", SourceExclude: "client"},
			want: []string{"src-spec-1", "src-spec-2", "src-impl-1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			got, err := resolveSelectorIDs(tt.opts, tt.flagIDs, tt.labelIDs, srcs, labels, &buf)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("want error %q, got nil; result=%v status=%q", tt.wantErr, got, buf.String())
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("want error containing %q, got %q", tt.wantErr, err.Error())
				}
				for _, sub := range tt.wantStatusContains {
					if !strings.Contains(buf.String(), sub) {
						t.Errorf("status missing %q\nfull status:\n%s", sub, buf.String())
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v\nstatus: %s", err, buf.String())
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %v\nwant %v\nstatus:\n%s", got, tt.want, buf.String())
			}
			for _, sub := range tt.wantStatusContains {
				if !strings.Contains(buf.String(), sub) {
					t.Errorf("status missing %q\nfull status:\n%s", sub, buf.String())
				}
			}
		})
	}
}
