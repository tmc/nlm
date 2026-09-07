package main

import (
	"io"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/tmc/nlm/notebooklm"
)

func TestLabelSelectors(t *testing.T) {
	sources := []sourceSummary{{ID: "a", Title: "a.txt"}, {ID: "b", Title: "b"}, {ID: "c", Title: "c"}}
	labels := []notebooklm.Label{
		{LabelID: "l1", Name: "same", SourceIDs: []string{"a", "stale"}},
		{LabelID: "l2", Name: "same", SourceIDs: []string{"b"}},
		{LabelID: "l3", Name: "other", SourceIDs: []string{"a"}},
	}
	for _, tt := range []struct {
		name                                  string
		opts                                  selectorOptions
		sourceIDs, labelIDs, excludeIDs, want []string
		wantErr                               string
	}{
		{name: "none", opts: selectorOptions{LabelNone: true}, want: []string{"c"}},
		{name: "none and matches", opts: selectorOptions{LabelNone: true, LabelMatch: "same"}, want: []string{"a", "b", "c"}},
		{name: "none with zero regex hits", opts: selectorOptions{LabelNone: true, LabelMatch: "never"}, want: []string{"c"}},
		{name: "exclude IDs", opts: selectorOptions{LabelMatch: "same", LabelExcludeIDs: "l1"}, excludeIDs: []string{"l1"}, want: []string{"b"}},
		{name: "exclude multiple labels", opts: selectorOptions{LabelMatch: "same", LabelExcludeIDs: "l3"}, excludeIDs: []string{"l3"}, want: []string{"b"}},
		{name: "exclusion alone", opts: selectorOptions{LabelExcludeIDs: "l1"}, excludeIDs: []string{"l1"}, want: []string{"b", "c"}},
		{name: "unknown source", opts: selectorOptions{SourceIDs: "a,bad"}, sourceIDs: []string{"a", "bad"}, wantErr: "unknown IDs: bad"},
		{name: "unknown label", opts: selectorOptions{LabelIDs: "l1,bad"}, labelIDs: []string{"l1", "bad"}, wantErr: "unknown IDs: bad"},
		{name: "unknown exclude", opts: selectorOptions{LabelExcludeIDs: "bad"}, excludeIDs: []string{"bad"}, wantErr: "unknown IDs: bad"},
		{name: "duplicate label names", opts: selectorOptions{LabelMatch: "same"}, want: []string{"a", "b"}},
		{name: "deduplicate IDs", opts: selectorOptions{SourceIDs: "a,a"}, sourceIDs: []string{"a", "a"}, want: []string{"a"}},
		{name: "regex metacharacters", opts: selectorOptions{SourceMatch: `^a\.txt$`}, want: []string{"a"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveSelectorIDs(tt.opts, tt.sourceIDs, tt.labelIDs, tt.excludeIDs, sources, labels, io.Discard)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error=%v", err)
				}
				return
			}
			if err != nil || !reflect.DeepEqual(got.IDs, tt.want) {
				t.Fatalf("got %+v, %v; want %v", got, err, tt.want)
			}
		})
	}
	// All sources labeled: label-none has zero hits, but the regex still wins.
	labels = append(labels, notebooklm.Label{LabelID: "l4", Name: "last", SourceIDs: []string{"c"}})
	got, err := resolveSelectorIDs(selectorOptions{LabelNone: true, LabelMatch: "same"}, nil, nil, nil, sources, labels, io.Discard)
	if err != nil || !reflect.DeepEqual(got.IDs, []string{"a", "b"}) {
		t.Fatalf("zero-hit none: %+v, %v", got, err)
	}
	if _, err := resolveSelectorIDs(selectorOptions{LabelNone: true}, nil, nil, nil, sources, labels, io.Discard); err == nil {
		t.Fatal("all-labeled selection did not fail")
	}
	if _, err := resolveSelectorIDs(selectorOptions{LabelNone: true}, nil, nil, nil, nil, nil, io.Discard); err == nil {
		t.Fatal("empty notebook did not fail")
	}
}

func TestSelectorStdinPreflight(t *testing.T) {
	input, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	defer writer.Close()
	old := os.Stdin
	os.Stdin = input
	defer func() { os.Stdin = old }()
	// Leave the pipe open: any attempted read would block. Shape validation
	// must complete before it consults either stdin or the nil client.
	for _, opts := range []selectorOptions{
		{SourceIDs: "-", LabelIDs: "-", Mode: "union"},
		{LabelIDs: "-", LabelExcludeIDs: "-"},
		{SourceIDs: "-", SourceMatch: "["},
	} {
		if _, err := resolveSourceSelectorsWithOptions(nil, "nb", opts); err == nil {
			t.Fatalf("accepted %+v", opts)
		}
	}
	if _, err := tryParseChatCommandForTest(t, "generate-chat", []string{"--prompt-file", "-", "--source-ids", "-", "nb"}, globalOptions{}); err == nil {
		t.Fatal("two stdin readers accepted")
	}
	if err := generateReport(nil, "nb", reportOptions{Selectors: selectorOptions{SourceIDs: "-"}}); err == nil {
		t.Fatal("report topics and IDs shared stdin")
	}
}

func TestLabelFlagsParsing(t *testing.T) {
	p := parseChatCommandForTest(t, "generate-chat", []string{"--label-none", "--label-exclude-ids", "l", "--label-match", "first", "--label-match", "last", "nb", "prompt"}, globalOptions{})
	opts := decodeSelectorOptions(p)
	if !opts.LabelNone || opts.LabelExcludeIDs != "l" || opts.LabelMatch != "last" {
		t.Fatalf("options=%+v", opts)
	}
}

func TestEmptySelectorFlagValues(t *testing.T) {
	for _, name := range []string{"source-ids", "label-ids", "label-exclude-ids", "source-match", "label-match", "source-exclude", "label-exclude", "selector-mode"} {
		if _, err := tryParseChatCommandForTest(t, "generate-chat", []string{"--" + name, "", "nb", "prompt"}, globalOptions{}); err == nil {
			t.Fatalf("empty --%s accepted", name)
		}
	}
}

func TestLabelExclusionIDsFromStdin(t *testing.T) {
	input, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	if _, err := writer.WriteString("l1\nl1\n"); err != nil {
		t.Fatal(err)
	}
	writer.Close()
	old := os.Stdin
	os.Stdin = input
	defer func() { os.Stdin = old }()
	ids, err := resolveIDList("-")
	if err != nil {
		t.Fatal(err)
	}
	got, err := resolveSelectorIDs(selectorOptions{LabelExcludeIDs: "-"}, nil, nil, ids, []sourceSummary{{ID: "a"}, {ID: "b"}}, []notebooklm.Label{{LabelID: "l1", SourceIDs: []string{"a"}}}, io.Discard)
	if err != nil || !reflect.DeepEqual(got.IDs, []string{"b"}) {
		t.Fatalf("got %+v, %v", got, err)
	}
}
