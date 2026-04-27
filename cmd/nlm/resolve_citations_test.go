package main

import (
	"errors"
	"testing"

	"github.com/tmc/nlm/internal/designreview"
	"github.com/tmc/nlm/internal/notebooklm/api"
)

func TestResolveCitationLocations(t *testing.T) {
	const txtar = "" +
		"-- main.go --\n" +
		"package main\n" +
		"\n" +
		"func Hello() string {\n" +
		"\treturn \"hi\"\n" +
		"}\n" +
		"-- README.md --\n" +
		"This is the readme.\n" +
		"It has two lines.\n"

	body := api.LoadSourceText{
		SourceID: "src_txtar",
		Title:    "project.txtar",
		Fragments: []api.TextFragment{
			{Start: 0, End: len(txtar), Text: txtar},
		},
	}

	helloOff := indexOf(txtar, "Hello") // member main.go
	readmeOff := indexOf(txtar, "two")  // member README.md

	cites := []api.Citation{
		{SourceIndex: 1, SourceID: "src_txtar", StartChar: helloOff, EndChar: helloOff + len("Hello()")},
		{SourceIndex: 2, SourceID: "src_txtar", StartChar: readmeOff, EndChar: readmeOff + len("two lines")},
		{SourceIndex: 3, SourceID: "src_other"}, // no body — skipped
	}

	load := func(id string) (api.LoadSourceText, error) {
		switch id {
		case "src_txtar":
			return body, nil
		case "src_other":
			return api.LoadSourceText{}, errors.New("not found")
		}
		return api.LoadSourceText{}, errors.New("unexpected id " + id)
	}

	got := resolveCitationLocations(load, cites)
	if len(got) != 2 {
		t.Fatalf("got %d locations, want 2: %+v", len(got), got)
	}

	// "Hello()" inside `func Hello() string {` lives on member main.go's
	// line 3, columns 6 through 12 (inclusive: "Hello()" is 7 chars; the
	// resolver reports end-column at the last char inside the span).
	wantHello := "main.go:3:6-12"
	if loc := got[keyFor(cites[0])]; loc != wantHello {
		t.Errorf("hello location = %q, want %q", loc, wantHello)
	}
	// "two lines" sits on README.md's line 2 starting at column 8.
	wantReadme := "README.md:2:8-16"
	if loc := got[keyFor(cites[1])]; loc != wantReadme {
		t.Errorf("readme location = %q, want %q", loc, wantReadme)
	}
}

// TestFormatLocation pins the editor-style file:line:col rendering across
// the shapes formatLocation handles.
func TestFormatLocation(t *testing.T) {
	cases := []struct {
		name string
		r    designreview.Resolved
		want string
	}{
		{"line only (column missing)", designreview.Resolved{File: "main.go", Line: 5}, "main.go:5"},
		{"line and col", designreview.Resolved{File: "main.go", Line: 5, Column: 7}, "main.go:5:7"},
		{"single line span", designreview.Resolved{File: "main.go", Line: 5, Column: 7, EndColumn: 12}, "main.go:5:7-12"},
		{"multi-line span", designreview.Resolved{File: "main.go", Line: 5, Column: 7, EndLine: 9, EndColumn: 4}, "main.go:5:7-9:4"},
		{"multi-line span, end col missing", designreview.Resolved{File: "main.go", Line: 5, Column: 7, EndLine: 9}, "main.go:5:7-9"},
		{"multi-line, both columns missing", designreview.Resolved{File: "main.go", Line: 5, EndLine: 9}, "main.go:5-9"},
		{"line zero degrades to file", designreview.Resolved{File: "main.go"}, "main.go"},
	}
	for _, tc := range cases {
		got := formatLocation(tc.r)
		if got != tc.want {
			t.Errorf("%s: formatLocation = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestResolveCitationLocationsNoLoader(t *testing.T) {
	if got := resolveCitationLocations(nil, []api.Citation{{SourceID: "x"}}); got != nil {
		t.Fatalf("nil loader should return nil, got %v", got)
	}
}

func TestResolveCitationLocationsNonTxtarSource(t *testing.T) {
	const plain = "Just a single-file source.\nNo txtar markers.\n"
	body := api.LoadSourceText{
		SourceID:  "src_plain",
		Title:     "plain.txt",
		Fragments: []api.TextFragment{{Start: 0, End: len(plain), Text: plain}},
	}
	load := func(string) (api.LoadSourceText, error) { return body, nil }

	got := resolveCitationLocations(load, []api.Citation{
		{SourceIndex: 1, SourceID: "src_plain", StartChar: 0, EndChar: 4},
	})
	if len(got) != 0 {
		t.Fatalf("non-txtar source should not produce locations, got %v", got)
	}
}

func indexOf(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
