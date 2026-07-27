package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/tmc/nlm/internal/designreview"
	"github.com/tmc/nlm/internal/notebooklm/api"
)

// TestResolveCitationLocations pins the txtar file:line resolution. Excerpts no
// longer come from here — the server ships them inline on the citation — so
// this only checks locations.
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

	// "Hello()" sits at member main.go's line 3, column 6.
	wantHello := "main.go:3:6"
	if loc := got[keyFor(cites[0])].Location; loc != wantHello {
		t.Errorf("hello location = %q, want %q", loc, wantHello)
	}
	// "two lines" sits on README.md's line 2, column 8.
	wantReadme := "README.md:2:8"
	if loc := got[keyFor(cites[1])].Location; loc != wantReadme {
		t.Errorf("readme location = %q, want %q", loc, wantReadme)
	}
}

// TestFormatLocation pins the vim/quickfix-clickable file:line:col rendering.
func TestFormatLocation(t *testing.T) {
	cases := []struct {
		name string
		r    designreview.Resolved
		want string
	}{
		{"line only (column missing)", designreview.Resolved{File: "main.go", Line: 5}, "main.go:5"},
		{"line and col", designreview.Resolved{File: "main.go", Line: 5, Column: 7}, "main.go:5:7"},
		{"end col is dropped", designreview.Resolved{File: "main.go", Line: 5, Column: 7, EndColumn: 12}, "main.go:5:7"},
		{"end line is dropped", designreview.Resolved{File: "main.go", Line: 5, Column: 7, EndLine: 9, EndColumn: 4}, "main.go:5:7"},
		{"line zero degrades to file", designreview.Resolved{File: "main.go"}, "main.go"},
	}
	for _, tc := range cases {
		got := formatLocation(tc.r)
		if got != tc.want {
			t.Errorf("%s: formatLocation = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// TestFormatLocationShortenAbsolutePath checks that an absolute path inside
// the current working directory is rendered as a relative path so the
// rendered citation is clickable from a terminal launched at that cwd.
func TestFormatLocationShortenAbsolutePath(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	abs := filepath.Join(cwd, "subdir", "file.go")
	got := formatLocation(designreview.Resolved{File: abs, Line: 5, Column: 7})
	want := "subdir/file.go:5:7"
	if got != want {
		t.Errorf("formatLocation(abs in cwd) = %q, want %q", got, want)
	}
}

func TestResolveCitationLocationsNoLoader(t *testing.T) {
	if got := resolveCitationLocations(nil, []api.Citation{{SourceID: "x"}}); got != nil {
		t.Fatalf("nil loader should return nil, got %v", got)
	}
}

// TestResolveCitationLocationsNonTxtarSource checks that a plain single-file
// source yields no entry: there is no txtar member to pin a file:line against,
// and excerpts no longer come from this path.
func TestResolveCitationLocationsNonTxtarSource(t *testing.T) {
	const plain = "Just a single-file source.\nNo txtar markers.\n"
	body := api.LoadSourceText{
		SourceID:  "src_plain",
		Title:     "plain.txt",
		Fragments: []api.TextFragment{{Start: 0, End: len(plain), Text: plain}},
	}
	load := func(string) (api.LoadSourceText, error) { return body, nil }

	cite := api.Citation{SourceIndex: 1, SourceID: "src_plain", StartChar: 0, EndChar: 4}
	got := resolveCitationLocations(load, []api.Citation{cite})
	if len(got) != 0 {
		t.Fatalf("non-txtar source has no location to resolve, got %v", got)
	}
}

// TestCitationContentKeyMatchesAcrossWhitespace pins the property chat-show's
// citation rehydration relies on: the locally-persisted answer text and the
// same answer refetched from GetConversationHistory can differ in whitespace
// (the history frame strips/rewrites newlines), so the match key must collapse
// whitespace and still agree. It must also distinguish different turns.
func TestCitationContentKeyMatchesAcrossWhitespace(t *testing.T) {
	persisted := "### Candidate Skills Triage List: Pass 3\n\nA rigorous third pass over the logs."
	history := "### Candidate Skills Triage List: Pass 3   A rigorous third pass over the logs."
	if citationContentKey(persisted) != citationContentKey(history) {
		t.Errorf("keys differ across whitespace:\n persisted=%q\n history  =%q",
			citationContentKey(persisted), citationContentKey(history))
	}

	other := "### Candidate Skills Triage List: Pass 2\n\nA different earlier pass."
	if citationContentKey(persisted) == citationContentKey(other) {
		t.Error("distinct turns collapsed to the same key")
	}

	// Empty content yields a stable (empty) key; the map lookup for a message
	// with no content simply misses and falls back to persisted citations.
	if citationContentKey("") != "" {
		t.Errorf("empty content key = %q, want empty", citationContentKey(""))
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
