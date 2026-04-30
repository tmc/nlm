package designreview

import (
	"os"
	"strings"
	"testing"

	"github.com/tmc/nlm/internal/notebooklm/api"
)

func TestReadNativeCitations(t *testing.T) {
	in := strings.NewReader(strings.Join([]string{
		`{"phase":"answer","text":"hello"}`,
		`{"phase":"citation","source_id":"src-1","title":"a.txt","start_char":3,"end_char":8,"confidence":0.9}`,
		`{"source_id":"src-2","start_char":10,"end_char":14}`,
		`{"phase":"done"}`,
	}, "\n"))
	got, err := ReadNativeCitations(in)
	if err != nil {
		t.Fatalf("ReadNativeCitations: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].SourceID != "src-1" || got[0].StartChar != 3 || got[0].EndChar != 8 {
		t.Fatalf("first citation = %+v", got[0])
	}
	if got[1].SourceID != "src-2" {
		t.Fatalf("second citation = %+v", got[1])
	}
}

func TestRenderChatAnswer(t *testing.T) {
	in := strings.NewReader(strings.Join([]string{
		`{"phase":"thinking","text":"hidden"}`,
		`{"phase":"answer","text":"hello "}`,
		`{"phase":"citation","source_id":"src-1","start_char":0,"end_char":5}`,
		`{"phase":"answer","text":"world"}`,
		`{"phase":"done"}`,
	}, "\n"))
	var b strings.Builder
	if err := RenderChatAnswer(&b, in); err != nil {
		t.Fatalf("RenderChatAnswer: %v", err)
	}
	if got := b.String(); got != "hello world" {
		t.Fatalf("answer = %q, want %q", got, "hello world")
	}
}

func TestResolvePlainText(t *testing.T) {
	body := api.LoadSourceText{
		SourceID: "src-1",
		Title:    "notes.txt",
		Fragments: []api.TextFragment{{
			Start: 0,
			End:   len([]rune("alpha\nbeta\ngamma")),
			Text:  "alpha\nbeta\ngamma",
		}},
	}
	start := strings.Index(body.Full(), "beta")
	got := Resolve(body, NativeCitation{
		SourceID:  body.SourceID,
		StartChar: start,
		EndChar:   start + len([]rune("beta")),
	})
	if got.Status != StatusOK {
		t.Fatalf("status = %s, want ok", got.Status)
	}
	if got.File != "notes.txt" {
		t.Fatalf("file = %q, want notes.txt", got.File)
	}
	if got.Line != 2 || got.Column != 1 {
		t.Fatalf("start = %d:%d, want 2:1", got.Line, got.Column)
	}
	if got.EndLine != 2 || got.EndColumn != 4 {
		t.Fatalf("end = %d:%d, want 2:4", got.EndLine, got.EndColumn)
	}
	if got.Snippet != "beta" {
		t.Fatalf("snippet = %q, want beta", got.Snippet)
	}
}

func TestResolveTxtar(t *testing.T) {
	const full = "" +
		"-- alpha.txt --\n" +
		"first line\n" +
		"second line\n" +
		"-- dir/beta.go --\n" +
		"package beta\n" +
		"func F() {}\n"
	body := api.LoadSourceText{
		SourceID: "src-1",
		Title:    ".",
		Fragments: []api.TextFragment{{
			Start: 0,
			End:   len([]rune(full)),
			Text:  full,
		}},
	}
	start := strings.Index(full, "second line")
	got := Resolve(body, NativeCitation{
		SourceID:  body.SourceID,
		StartChar: start,
		EndChar:   start + len([]rune("second line")),
	})
	if got.Status != StatusOK {
		t.Fatalf("status = %s, want ok", got.Status)
	}
	if got.File != "alpha.txt" {
		t.Fatalf("file = %q, want alpha.txt", got.File)
	}
	if got.Line != 2 || got.Column != 1 {
		t.Fatalf("start = %d:%d, want 2:1", got.Line, got.Column)
	}
	if got.EndLine != 2 || got.EndColumn != 11 {
		t.Fatalf("end = %d:%d, want 2:11", got.EndLine, got.EndColumn)
	}
	if got.Snippet != "second line" {
		t.Fatalf("snippet = %q, want second line", got.Snippet)
	}
}

func TestResolveHeaderSpan(t *testing.T) {
	const full = "" +
		"-- alpha.txt --\n" +
		"first line\n" +
		"-- dir/beta.go --\n" +
		"package beta\n"
	body := api.LoadSourceText{
		SourceID: "src-1",
		Fragments: []api.TextFragment{{
			Start: 0,
			End:   len([]rune(full)),
			Text:  full,
		}},
	}
	start := strings.Index(full, "-- dir/beta.go --")
	got := Resolve(body, NativeCitation{
		SourceID:  body.SourceID,
		StartChar: start,
		EndChar:   start + len([]rune("-- dir")),
	})
	if got.Status != StatusHeaderSpan {
		t.Fatalf("status = %s, want %s", got.Status, StatusHeaderSpan)
	}
	if got.File != "dir/beta.go" {
		t.Fatalf("file = %q, want dir/beta.go", got.File)
	}
	if got.Line != 0 || got.Column != 0 {
		t.Fatalf("unexpected coordinates = %d:%d", got.Line, got.Column)
	}
}

// TestResolveTxtarHeadersStripped covers the real-world shape produced when
// the server collapses newlines and renders multiple members on one "line":
// `-- a.md --content...-- b.md --content...`. Headers terminated only by
// content (rather than newline/space/EOF) are not real txtar headers, so
// the resolver must not greedy-match across them. The citation falls back
// to plain-text reporting against the source's title.
func TestResolveTxtarHeadersStripped(t *testing.T) {
	const full = "-- a.md --content one -- b.md --content two\n"
	body := api.LoadSourceText{
		SourceID: "src-1",
		Title:    "stripped.txtar",
		Fragments: []api.TextFragment{{
			Start: 0,
			End:   len([]rune(full)),
			Text:  full,
		}},
	}
	start := strings.Index(full, "content one")
	got := Resolve(body, NativeCitation{
		SourceID:  body.SourceID,
		StartChar: start,
		EndChar:   start + len([]rune("content one")),
	})
	if got.Status != StatusOK {
		t.Fatalf("status = %s, want ok", got.Status)
	}
	if got.File != "stripped.txtar" {
		t.Fatalf("file = %q, want stripped.txtar (no member matched)", got.File)
	}
}

// TestResolveTxtarHeaderTrailingContent is the structural pin: a line that
// looks like `-- name --extra` is NOT a valid header (the closing `--` must
// be followed by a header-boundary or EOF), so the resolver should treat
// the whole source as plain text. Previous behavior greedy-matched and
// produced names like `a.md --extra blah`.
func TestResolveTxtarHeaderTrailingContent(t *testing.T) {
	const full = "" +
		"-- alpha.txt --first line\n" +
		"second line\n"
	body := api.LoadSourceText{
		SourceID:  "src-1",
		Title:     "x.txt",
		Fragments: []api.TextFragment{{Start: 0, End: len([]rune(full)), Text: full}},
	}
	start := strings.Index(full, "first line")
	got := Resolve(body, NativeCitation{
		SourceID:  body.SourceID,
		StartChar: start,
		EndChar:   start + len([]rune("first line")),
	})
	if got.Status != StatusOK {
		t.Fatalf("status = %s, want ok", got.Status)
	}
	if got.File != "x.txt" {
		t.Fatalf("file = %q, want x.txt (no greedy header match)", got.File)
	}
}

func TestResolveAllCachesSourceLoads(t *testing.T) {
	body := api.LoadSourceText{
		SourceID: "src-1",
		Fragments: []api.TextFragment{{
			Start: 0,
			End:   len([]rune("hello world")),
			Text:  "hello world",
		}},
	}
	loads := 0
	got, err := ResolveAll(func(sourceID string) (api.LoadSourceText, error) {
		loads++
		return body, nil
	}, []NativeCitation{
		{SourceID: "src-1", StartChar: 0, EndChar: 5},
		{SourceID: "src-1", StartChar: 6, EndChar: 11},
	})
	if err != nil {
		t.Fatalf("ResolveAll: %v", err)
	}
	if loads != 1 {
		t.Fatalf("loads = %d, want 1", loads)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
}

func TestResolvedAsCitation(t *testing.T) {
	got := ResolvedAsCitation(Resolved{
		SourceID:   "src-1",
		File:       "dir/file.go",
		Line:       12,
		Column:     7,
		EndLine:    12,
		EndColumn:  11,
		Status:     StatusOK,
		Confidence: 0.87,
		Snippet:    "hello world",
	}, "/repo")
	if got.Raw != "dir/file.go:12:7" {
		t.Fatalf("raw = %q", got.Raw)
	}
	if got.Match != "/repo/dir/file.go" {
		t.Fatalf("match = %q", got.Match)
	}
	if got.Snippet != "hello world" || got.Context != "hello world" {
		t.Fatalf("snippet/context = %+v", got)
	}
}

func TestResolveFixture(t *testing.T) {
	path := "/tmp/hizojc-txtar.json"
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("%s not present; skip live fixture", path)
	}
	body, err := api.DecodeLoadSourceText(raw)
	if err != nil {
		t.Fatalf("DecodeLoadSourceText: %v", err)
	}
	resolver := newSourceResolver(body)
	if len(resolver.members) == 0 {
		t.Fatalf("fixture decoded without txtar headers")
	}
	member := resolver.members[0]
	relStart, line := firstNonEmptyLine(resolver.text[member.BodyStart:member.BodyEnd])
	if relStart < 0 {
		t.Fatalf("fixture member %q has no non-empty line", member.Name)
	}
	start := member.BodyStart + relStart
	end := start + len([]rune(line))
	got := resolver.Resolve(NativeCitation{
		SourceID:  body.SourceID,
		StartChar: start,
		EndChar:   end,
	})
	if got.Status != StatusOK {
		t.Fatalf("status = %s, want ok", got.Status)
	}
	if got.File != member.Name {
		t.Fatalf("file = %q, want %q", got.File, member.Name)
	}
	if displaySnippet(got.Snippet) != displaySnippet(line) {
		t.Fatalf("snippet = %q, want %q", displaySnippet(got.Snippet), displaySnippet(line))
	}
}

func firstNonEmptyLine(text []rune) (int, string) {
	lineStart := 0
	for i, r := range text {
		if r != '\n' {
			continue
		}
		line := strings.TrimSpace(string(text[lineStart:i]))
		if line != "" {
			return lineStart, line
		}
		lineStart = i + 1
	}
	line := strings.TrimSpace(string(text[lineStart:]))
	if line == "" {
		return -1, ""
	}
	return lineStart, line
}
