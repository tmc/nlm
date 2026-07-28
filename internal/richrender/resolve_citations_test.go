package richrender

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/tmc/nlm/internal/notebooklm/api"
	"github.com/tmc/nlm/internal/sourcecite"
)

// TestResolveCitationLocations pins excerpt-validated txtar file:line
// resolution.
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
		{
			SourceIndex: 1, SourceID: "chunk_hello", ParentSourceID: "src_txtar",
			StartChar: 4, EndChar: 12, SourceStart: helloOff, SourceEnd: helloOff + len("Hello()"),
			Excerpt: "Hello()",
		},
		{
			SourceIndex: 2, SourceID: "chunk_readme", ParentSourceID: "src_txtar",
			StartChar: 20, EndChar: 29, SourceStart: readmeOff, SourceEnd: readmeOff + len("two lines"),
			Excerpt: "two lines",
		},
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

	got := resolveCitationLocations(load, cites, nil)
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

func TestResolveOneCitationRejectsExcerptMismatch(t *testing.T) {
	const txtar = "-- main.go --\npackage main\n"
	body := api.LoadSourceText{
		SourceID:  "src",
		Title:     "project.txtar",
		Fragments: []api.TextFragment{{Start: 0, End: len(txtar), Text: txtar}},
	}
	start := indexOf(txtar, "package")
	cite := api.Citation{
		SourceID:    "src",
		SourceStart: start,
		SourceEnd:   start + len("package"),
		Excerpt:     "different text",
	}
	if got, ok, _ := resolveOneCitation(body, cite); ok {
		t.Fatalf("resolveOneCitation = %+v, true; want excerpt mismatch", got)
	}
}

func TestResolveOneCitationUsesCompactProjection(t *testing.T) {
	const (
		first  = "-- main.go --\n"
		second = "package main\n"
	)
	body := api.LoadSourceText{
		SourceID: "src",
		Title:    "project.txtar",
		Fragments: []api.TextFragment{
			{Start: 100, End: 100 + len(first), Text: first},
			{Start: 500, End: 500 + len(second), Text: second},
		},
	}
	start := len(first)
	cite := api.Citation{
		SourceID:    "src",
		SourceStart: start,
		SourceEnd:   start + len("package"),
		Excerpt:     "package",
	}
	got, ok, _ := resolveOneCitation(body, cite)
	if !ok {
		t.Fatal("resolveOneCitation did not use compact projection")
	}
	if got.Location != "main.go:1:1" {
		t.Fatalf("location = %q, want main.go:1:1", got.Location)
	}
}

// TestFormatLocation pins the vim/quickfix-clickable file:line:col rendering.
func TestFormatLocation(t *testing.T) {
	cases := []struct {
		name string
		r    sourcecite.Resolved
		want string
	}{
		{"line only (column missing)", sourcecite.Resolved{File: "main.go", Line: 5, LineExact: true}, "main.go:5"},
		{"line and col", sourcecite.Resolved{File: "main.go", Line: 5, Column: 7, LineExact: true}, "main.go:5:7"},
		{"end col is dropped", sourcecite.Resolved{File: "main.go", Line: 5, Column: 7, EndColumn: 12, LineExact: true}, "main.go:5:7"},
		{"end line is dropped", sourcecite.Resolved{File: "main.go", Line: 5, Column: 7, EndLine: 9, EndColumn: 4, LineExact: true}, "main.go:5:7"},
		{"line zero degrades to file", sourcecite.Resolved{File: "main.go"}, "main.go"},
		{
			"loose member offset",
			sourcecite.Resolved{File: "main.go", MemberOffset: 41, OffsetKnown: true},
			"main.go (+41 chars)",
		},
		{
			"multi-member span",
			sourcecite.Resolved{Members: []string{"a.py", "b.py", "f.py"}},
			"a.py … f.py (3 files)",
		},
	}
	for _, tc := range cases {
		got := formatLocation(tc.r)
		if got != tc.want {
			t.Errorf("%s: formatLocation = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestResolveOneCitationMultiMemberSpan(t *testing.T) {
	const txtar = "-- a.py --\none\n-- b.py --\ntwo\n"
	body := api.LoadSourceText{
		SourceID:  "src",
		Title:     "project.txtar",
		Fragments: []api.TextFragment{{Start: 0, End: len(txtar), Text: txtar}},
	}
	start := indexOf(txtar, "one")
	end := indexOf(txtar, "two") + len("two")
	cite := api.Citation{
		SourceID:    "src",
		SourceStart: start,
		SourceEnd:   end,
		Excerpt:     txtar[start:end],
	}
	got, ok, _ := resolveOneCitation(body, cite)
	if !ok {
		t.Fatal("resolveOneCitation did not retain multi-member span")
	}
	if got.Location != "a.py … b.py (2 files)" {
		t.Fatalf("location = %q", got.Location)
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
	got := formatLocation(sourcecite.Resolved{File: abs, Line: 5, Column: 7, LineExact: true})
	want := "subdir/file.go:5:7"
	if got != want {
		t.Errorf("formatLocation(abs in cwd) = %q, want %q", got, want)
	}
}

func TestResolveCitationLocationsNoLoader(t *testing.T) {
	if got := resolveCitationLocations(nil, []api.Citation{{SourceID: "x"}}, nil); got != nil {
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
	got := resolveCitationLocations(load, []api.Citation{cite}, nil)
	if len(got) != 0 {
		t.Fatalf("non-txtar source has no location to resolve, got %v", got)
	}
}

func TestResolveCitationLocationsDebugReasons(t *testing.T) {
	const txtar = "-- main.go --\npackage main\n"
	body := api.LoadSourceText{
		SourceID:  "src",
		Title:     "project.txtar",
		Fragments: []api.TextFragment{{Start: 0, End: len(txtar), Text: txtar}},
	}
	plain := api.LoadSourceText{
		SourceID:  "plain",
		Title:     "notes.txt",
		Fragments: []api.TextFragment{{Start: 0, End: 5, Text: "notes"}},
	}
	load := func(id string) (api.LoadSourceText, error) {
		switch id {
		case "missing":
			return api.LoadSourceText{}, errors.New("missing")
		case "src":
			return body, nil
		case "plain":
			return plain, nil
		default:
			return api.LoadSourceText{}, errors.New("unexpected")
		}
	}
	start := indexOf(txtar, "package")
	cites := []api.Citation{
		{SourceIndex: 1, SourceID: "missing"},
		{SourceIndex: 1, SourceID: "missing"}, // duplicate
		{
			SourceIndex: 2,
			SourceID:    "src",
			SourceStart: start,
			SourceEnd:   start + len("package"),
			Excerpt:     "different",
		},
		{
			SourceIndex: 3,
			SourceID:    "plain",
			SourceStart: 0,
			SourceEnd:   5,
			Excerpt:     "notes",
		},
		{
			SourceIndex: 4,
			SourceID:    "src",
			SourceStart: 100,
			SourceEnd:   101,
			Excerpt:     "x",
		},
		{
			SourceIndex: 5,
			SourceID:    "src",
			SourceStart: 0,
			SourceEnd:   len("-- main"),
			Excerpt:     "-- main",
		},
		{SourceIndex: 6},
	}
	var debug bytes.Buffer
	resolveCitationLocations(load, cites, &debug)
	const want = "" +
		"nlm: citation 1: load error\n" +
		"nlm: citation 2: excerpt mismatch\n" +
		"nlm: citation 3: no members\n" +
		"nlm: citation 4: offset miss\n" +
		"nlm: citation 5: header span\n" +
		"nlm: citation 6: title-only\n"
	if got := debug.String(); got != want {
		t.Fatalf("debug output = %q, want %q", got, want)
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
