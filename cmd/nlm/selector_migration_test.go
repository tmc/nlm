package main

import (
	"context"
	"io"
	"net/http"
	"reflect"
	"testing"

	"github.com/tmc/nlm/notebooklm"
)

func TestSelectorShapeErrorsBeforeRPC(t *testing.T) {
	for _, tt := range []struct {
		name string
		opts selectorOptions
	}{
		{"mixed", selectorOptions{SourceMatch: "foo", LabelMatch: "bar"}},
		{"invalid mode", selectorOptions{Mode: "bogus", SourceIDs: "s"}},
		{"mode alone", selectorOptions{Mode: "union"}},
		{"invalid regex", selectorOptions{SourceMatch: "["}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tr := &chatRejectTransport{}
			c := notebooklm.New(notebooklm.Credentials{}, notebooklm.WithHTTPClient(&http.Client{Transport: tr}))
			if _, err := resolveSourceSelectorsWithOptions(c, "nb", tt.opts); err == nil {
				t.Fatal("accepted invalid shape")
			}
			if err := generateReport(c, "nb", reportOptions{Instructions: "must not be sent", Selectors: tt.opts}); err == nil {
				t.Fatal("report accepted invalid shape")
			}
			if err := appCreateCall(appCreateArgs{NotebookID: "nb", Options: appCreateOptions{Type: "mindmap", Selectors: tt.opts}})(context.Background(), c); err == nil {
				t.Fatal("app accepted invalid shape")
			}
			if tr.calls.Load() != 0 {
				t.Fatalf("issued %d RPCs", tr.calls.Load())
			}
		})
	}
}

func TestSelectorUnionActiveDimensions(t *testing.T) {
	sources := []sourceSummary{{ID: "s1", Title: "foo"}, {ID: "s2", Title: "bar"}, {ID: "s3", Title: "other"}}
	labels := []notebooklm.Label{{LabelID: "l", Name: "tag", SourceIDs: []string{"s2"}}}
	for _, tt := range []struct {
		name string
		opts selectorOptions
		want []string
	}{
		{"source", selectorOptions{Mode: "union", SourceMatch: "foo"}, []string{"s1"}},
		{"label", selectorOptions{Mode: "union", LabelMatch: "tag"}, []string{"s2"}},
		{"mixed", selectorOptions{Mode: "union", SourceMatch: "foo", LabelMatch: "tag"}, []string{"s1", "s2"}},
		{"excludes", selectorOptions{Mode: "union", SourceExclude: "other"}, []string{"s1", "s2"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveSelectorIDs(tt.opts, nil, nil, nil, sources, labels, io.Discard)
			if err != nil || !reflect.DeepEqual(got.IDs, tt.want) {
				t.Fatalf("got %+v, %v; want %v", got, err, tt.want)
			}
		})
	}
}

func TestSelectorMigrationParsing(t *testing.T) {
	for _, args := range [][]string{
		{"--source-match", "foo", "--label-match", "bar", "nb", "prompt"},
		{"--selector-mode", "bogus", "--source-ids", "s", "nb", "prompt"},
		{"--selector-mode", "union", "nb", "prompt"},
	} {
		if _, err := tryParseChatCommandForTest(t, "generate-chat", args, globalOptions{}); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
	p := parseChatCommandForTest(t, "generate-chat", []string{"--selector-mode", "union", "--source-match", "foo", "--label-match", "bar", "nb", "prompt"}, globalOptions{})
	if decodeSelectorOptions(p).Mode != "union" {
		t.Fatal("mode not decoded")
	}
	if err := parseCreateCommandErrorForTest(t, "source-guide", []string{"--source-match", "foo", "nb", "s"}); err == nil {
		t.Fatal("positional IDs override selectors")
	}
}
