package sourcecite

import (
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/tmc/nlm/internal/notebooklm/api"
)

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

func TestResolveCitationProjection(t *testing.T) {
	const (
		header   = "-- alpha.txt --\n"
		bodyText = "first line\nsecond line\n"
	)
	body := api.LoadSourceText{
		SourceID: "src-1",
		Title:    "project.txtar",
		Fragments: []api.TextFragment{
			{Start: 100, End: 100 + len(header), Text: header},
			{Start: 500, End: 500 + len(bodyText), Text: bodyText},
		},
	}
	start := len(header) + strings.Index(bodyText, "second line")
	got := ResolveCitation(body, NativeCitation{
		SourceID:  body.SourceID,
		StartChar: start,
		EndChar:   start + len("second line"),
	}, "second \n\t line")
	if got.Status != StatusOK {
		t.Fatalf("status = %s (%s), want ok", got.Status, got.Reason)
	}
	if got.Projection != "compact" {
		t.Fatalf("projection = %q, want compact", got.Projection)
	}
	if got.File != "alpha.txt" || got.Line != 2 || got.Column != 1 {
		t.Fatalf("resolved = %+v, want alpha.txt:2:1", got)
	}
}

func TestResolveCitationFailsClosed(t *testing.T) {
	const text = "-- alpha.txt --\nfirst line\n"
	body := api.LoadSourceText{
		SourceID:  "src-1",
		Title:     "project.txtar",
		Fragments: []api.TextFragment{{Start: 0, End: len(text), Text: text}},
	}
	tests := []struct {
		name    string
		start   int
		end     int
		excerpt string
		reason  string
	}{
		{"missing excerpt", len("-- alpha.txt --\n"), len("-- alpha.txt --\nfirst"), "", "citation has no excerpt"},
		{"mismatch", len("-- alpha.txt --\n"), len("-- alpha.txt --\nfirst"), "other text", "citation excerpt does not match source coordinates"},
		{"offset miss", 100, 110, "other text", "citation coordinates outside source projections"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := ResolveCitation(body, NativeCitation{
				SourceID:  body.SourceID,
				StartChar: test.start,
				EndChar:   test.end,
			}, test.excerpt)
			if got.Status != StatusOffsetMiss || got.Reason != test.reason {
				t.Fatalf("resolved = %+v, want offset miss %q", got, test.reason)
			}
		})
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

func TestResolveMultiMemberSpan(t *testing.T) {
	const full = "" +
		"-- alpha.txt --\n" +
		"first line\n" +
		"-- dir/beta.go --\n" +
		"package beta\n" +
		"-- gamma.md --\n" +
		"last line\n"
	body := api.LoadSourceText{
		SourceID:  "src-1",
		Fragments: []api.TextFragment{{Start: 0, End: len(full), Text: full}},
	}
	start := strings.Index(full, "first line")
	end := strings.Index(full, "last line") + len("last")
	got := Resolve(body, NativeCitation{
		SourceID:  body.SourceID,
		StartChar: start,
		EndChar:   end,
	})
	if got.Status != StatusHeaderSpan {
		t.Fatalf("status = %s, want %s", got.Status, StatusHeaderSpan)
	}
	want := []string{"alpha.txt", "dir/beta.go", "gamma.md"}
	if !slices.Equal(got.Members, want) {
		t.Fatalf("members = %q, want %q", got.Members, want)
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

func TestScanTxtarMembersCollapsedFirstHeader(t *testing.T) {
	// hizoJc returned the valid header and its body as adjacent fragments:
	// "-- ...bug_report.md --" then "name: Bug report...". This is a
	// read-side recovery test, not an accepted txtar authoring form.
	const text = "" +
		"-- .github/ISSUE_TEMPLATE/bug_report.md --name: Bug report " +
		"-- cmd/one.go -- package one " +
		"-- internal/two.go -- package two"
	got := scanTxtarMembers([]rune(text))
	if len(got) != 3 {
		t.Fatalf("members = %+v, want 3", got)
	}
	if got[0].Name != ".github/ISSUE_TEMPLATE/bug_report.md" ||
		got[0].HeaderStart != 0 ||
		got[0].BodyStart != len("-- .github/ISSUE_TEMPLATE/bug_report.md --") {
		t.Fatalf("first member = %+v", got[0])
	}
}

func TestResolveCollapsedTxtarUsesMemberOffset(t *testing.T) {
	const text = "" +
		"-- first.md --" + // adjacent hizoJc fragments: header, then body
		"body one " +
		"-- cmd/one.go -- package one " +
		"-- internal/two.go -- package two"
	body := api.LoadSourceText{
		SourceID:  "src",
		Title:     "project.txtar",
		Fragments: []api.TextFragment{{Start: 0, End: len(text), Text: text}},
	}
	start := strings.Index(text, "package one")
	got := Resolve(body, NativeCitation{
		SourceID:  body.SourceID,
		StartChar: start,
		EndChar:   start + len("package"),
	})
	if got.Status != StatusOK || got.File != "cmd/one.go" {
		t.Fatalf("resolved = %+v", got)
	}
	if got.LineExact || !got.OffsetKnown || got.MemberOffset != 0 {
		t.Fatalf("coordinates = %+v, want zero-based loose member offset", got)
	}
}

func TestScanTxtarMembersCollapsedFirstHeaderRequiresCorroboration(t *testing.T) {
	tests := []string{
		"-- first.md --body -- second.md -- body",
		"-- not a path --body -- second.md -- body -- third.md -- body",
	}
	for _, text := range tests {
		if got := scanTxtarMembers([]rune(text)); len(got) != 0 {
			t.Errorf("scanTxtarMembers(%q) = %+v, want no members", text, got)
		}
	}
}

func TestResolveFixture(t *testing.T) {
	path := os.Getenv("NLM_HIZOJC_FIXTURE")
	if path == "" {
		t.Skip("NLM_HIZOJC_FIXTURE is not set")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	body, err := api.DecodeLoadSourceText(raw)
	if err != nil {
		t.Fatalf("DecodeLoadSourceText: %v", err)
	}
	resolver := newSourceResolver(body)
	if len(resolver.members) == 0 {
		t.Fatalf("fixture decoded without txtar headers")
	}
	var member txtarMember
	relStart := -1
	var line string
	for _, candidate := range resolver.members {
		relStart, line = firstNonEmptyLine(resolver.text[candidate.BodyStart:candidate.BodyEnd])
		if relStart >= 0 {
			member = candidate
			break
		}
	}
	if relStart < 0 {
		t.Fatal("fixture has no non-empty txtar member")
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
