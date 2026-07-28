package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/tmc/nlm/internal/notebooklm/api"
)

func imageSourceBody() api.LoadSourceText {
	return api.LoadSourceText{
		SourceID: "source-1",
		Title:    "paper.pdf",
		Fragments: []api.TextFragment{
			{Start: 0, End: 1, Text: "A"},
			{Start: 1, End: 2, ImageURL: "https://example.test/image", ImageID: "image-1"},
			{Start: 2, End: 3, Text: "B"},
		},
	}
}

func testSourceImageFetcher(imageURL string) ([]byte, string, error) {
	if imageURL != "https://example.test/image" {
		return nil, "", fmt.Errorf("unexpected image URL %q", imageURL)
	}
	return []byte("image"), "image/png", nil
}

func TestWriteSourceRead_DefaultPreservesText(t *testing.T) {
	body := imageSourceBody()
	var out bytes.Buffer
	if err := writeSourceRead(&out, body, globalOptions{}, nil); err != nil {
		t.Fatal(err)
	}
	if got, want := out.String(), "A B"; got != want {
		t.Errorf("default read = %q, want %q", got, want)
	}
}

// A source whose index dropped a region leaves a wide offset gap between
// fragments. Full pads it with one space per missing offset to keep citation
// coordinates aligned; the default reading view must instead collapse the gap
// into a reading break so the dropped region does not surface as a run of
// blanks. Contiguous fragments are unchanged.
func TestWriteSourceRead_DefaultCollapsesDroppedGap(t *testing.T) {
	body := api.LoadSourceText{
		SourceID: "source-1",
		Title:    "code.go",
		Fragments: []api.TextFragment{
			{Start: 0, End: 24, Text: "func Add(a, b int) int {"},
			{Start: 71, End: 72, Text: "}"},
		},
	}
	var out bytes.Buffer
	if err := writeSourceRead(&out, body, globalOptions{}, nil); err != nil {
		t.Fatal(err)
	}
	if got, want := out.String(), "func Add(a, b int) int {\n\n}"; got != want {
		t.Errorf("default read = %q, want %q", got, want)
	}
	// Full stays offset-faithful for the citation resolver: the 47-offset gap
	// remains 47 padding spaces, so len(Full) still equals the last End.
	if got := len(body.Full()); got != 72 {
		t.Errorf("Full length = %d, want 72 (offset-faithful)", got)
	}
}

func TestWriteSourceRead_MarkdownIncludesImage(t *testing.T) {
	body := imageSourceBody()
	var out bytes.Buffer
	if err := writeSourceRead(&out, body, globalOptions{sourceReadMarkdown: true}, testSourceImageFetcher); err != nil {
		t.Fatal(err)
	}
	if got, want := out.String(), "A![](data:image/png;base64,aW1hZ2U=)B"; got != want {
		t.Errorf("markdown read = %q, want %q", got, want)
	}
}

func TestWriteSourceRead_HTMLIncludesImageAndMathJax(t *testing.T) {
	body := imageSourceBody()
	body.Title = "<paper>"
	var out bytes.Buffer
	if err := writeSourceRead(&out, body, globalOptions{sourceReadHTML: true}, testSourceImageFetcher); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{
		"<!doctype html>",
		"MathJax",
		"<title>&lt;paper&gt;</title>",
		`<img alt="" src="data:image/png;base64,aW1hZ2U=">`,
		"<p>A</p>",
		"<p>B</p>",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("HTML does not contain %q:\n%s", want, got)
		}
	}
}

func TestSourceReadImageCaptionBecomesAltText(t *testing.T) {
	body := api.LoadSourceText{Fragments: []api.TextFragment{
		{Start: 0, End: 1, ImageURL: "https://example.test/image", ImageID: "image-1"},
		{Start: 1, End: 35, Text: "Figure 1: The Transformer architecture."},
	}}
	markdown, err := sourceReadMarkdown(body, testSourceImageFetcher)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(markdown, "![Figure 1: The Transformer architecture.](data:image/png;base64,aW1hZ2U=)") {
		t.Errorf("markdown image alt = %q", markdown)
	}
	html, err := sourceReadHTML(body, testSourceImageFetcher)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, `alt="Figure 1: The Transformer architecture."`) {
		t.Errorf("HTML image alt = %q", html)
	}
}

func TestSourceReadHTMLRendersTable(t *testing.T) {
	body := api.LoadSourceText{Fragments: []api.TextFragment{
		{Start: 0, End: 16, Text: "| Model | BLEU |"},
		{Start: 17, End: 30, Text: "| --- | --- |"},
		{Start: 31, End: 48, Text: "| ByteNet | 23.75 |"},
	}}
	got, err := sourceReadHTML(body, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"<table>", "<th>Model</th>", "<td>ByteNet</td>", "<td>23.75</td>"} {
		if !strings.Contains(got, want) {
			t.Errorf("HTML table does not contain %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "---") {
		t.Errorf("HTML table includes divider row:\n%s", got)
	}
}

func TestNormalizeMathNoise(t *testing.T) {
	if got, want := normalizeMathNoise("𝑧 subscript 1 + 𝑧 superscript n"), "$𝑧_{1} + 𝑧^{n}$"; got != want {
		t.Errorf("normalizeMathNoise = %q, want %q", got, want)
	}
	if got, want := normalizeMathNoise("a subscript is a label"), "a subscript is a label"; got != want {
		t.Errorf("normalizeMathNoise prose = %q, want %q", got, want)
	}
	if got, want := normalizeMathNoise("𝑧 subscript 1\n𝑧 superscript n"), "$$\n𝑧_{1}\n𝑧^{n}\n$$"; got != want {
		t.Errorf("normalizeMathNoise block = %q, want %q", got, want)
	}
}

func TestSourceReadPresentationGaps(t *testing.T) {
	body := api.LoadSourceText{Fragments: []api.TextFragment{
		{Start: 0, End: 1, Text: "A"},
		{Start: 3, End: 4, Text: "B"},
		{Start: 5, End: 6, Text: "C"},
	}}
	markdown, err := sourceReadMarkdown(body, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := markdown, "A\n\nB C"; got != want {
		t.Errorf("markdown gaps = %q, want %q", got, want)
	}
	html, err := sourceReadHTML(body, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"<p>A</p>", "<p>B C</p>"} {
		if !strings.Contains(html, want) {
			t.Errorf("HTML gaps does not contain %q:\n%s", want, html)
		}
	}
}

func TestSourceReadPresentationBlocks(t *testing.T) {
	body := api.LoadSourceText{Fragments: []api.TextFragment{
		{Start: 0, End: 5, Text: "first"},
		{Start: 5, End: 11, Text: "second", BlockStart: true},
	}}
	markdown, err := sourceReadMarkdown(body, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := markdown, "first\n\nsecond"; got != want {
		t.Errorf("markdown blocks = %q, want %q", got, want)
	}
	html, err := sourceReadHTML(body, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"<p>first</p>", "<p>second</p>"} {
		if !strings.Contains(html, want) {
			t.Errorf("HTML blocks does not contain %q:\n%s", want, html)
		}
	}
}

func TestSourceReadRendersListMarkers(t *testing.T) {
	body := api.LoadSourceText{Fragments: []api.TextFragment{
		{Start: 0, End: 5, Text: "first", ListMarker: "•"},
		{Start: 5, End: 11, Text: "second", ListMarker: "•"},
	}}
	markdown, err := sourceReadMarkdown(body, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := markdown, "- first\n- second\n"; got != want {
		t.Errorf("markdown list = %q, want %q", got, want)
	}
	html, err := sourceReadHTML(body, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, "<ul><li>first</li><li>second</li></ul>") {
		t.Errorf("HTML list = %q", html)
	}
}

func TestSourceReadPreservesInlineStyle(t *testing.T) {
	body := api.LoadSourceText{Fragments: []api.TextFragment{
		{Start: 0, End: 4, Text: "bold", Bold: true},
		{Start: 4, End: 10, Text: "italic", Italic: true},
	}}
	markdown, err := sourceReadMarkdown(body, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := markdown, "**bold**_italic_"; got != want {
		t.Errorf("markdown style = %q, want %q", got, want)
	}
	html, err := sourceReadHTML(body, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, "<strong>bold</strong><em>italic</em>") {
		t.Errorf("HTML style = %q", html)
	}
}

func TestSourceReadMarkdownPreservesTableRows(t *testing.T) {
	body := api.LoadSourceText{Fragments: []api.TextFragment{
		{Start: 0, End: 16, Text: "| Model | BLEU |"},
		{Start: 17, End: 30, Text: "| --- | --- |"},
		{Start: 31, End: 48, Text: "| ByteNet | 23.75 |"},
	}}
	got, err := sourceReadMarkdown(body, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := "| Model | BLEU |\n| --- | --- |\n| ByteNet | 23.75 |"
	if got != want {
		t.Errorf("markdown table = %q, want %q", got, want)
	}
}

func TestSourceReadRendersCodeBlocks(t *testing.T) {
	body := api.LoadSourceText{Fragments: []api.TextFragment{{
		Start:    0,
		End:      13,
		Text:     "package main\n",
		Code:     true,
		Language: "go",
	}}}
	if got := sourceReadText(body); got != "package main\n" {
		t.Fatalf("text = %q", got)
	}
	markdown, err := sourceReadMarkdown(body, nil)
	if err != nil {
		t.Fatal(err)
	}
	if markdown != "```go\npackage main\n```\n\n" {
		t.Fatalf("markdown = %q", markdown)
	}
	document, err := sourceReadHTML(body, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(document, `<pre><code class="language-go">package main`+"\n"+`</code></pre>`) {
		t.Fatalf("HTML code block missing:\n%s", document)
	}
}

func TestSourceReadSeparatesTxtarMembers(t *testing.T) {
	body := api.LoadSourceText{Fragments: []api.TextFragment{
		{Start: 0, End: 10, Text: "-- a.go --"},
		{Start: 10, End: 19, Text: "package a"},
		{Start: 19, End: 29, Text: "-- b.go --"},
		{Start: 29, End: 38, Text: "package b"},
	}}
	const want = "-- a.go --\n\npackage a\n\n-- b.go --\n\npackage b"
	if got := sourceReadText(body); got != want {
		t.Fatalf("text = %q, want %q", got, want)
	}
	markdown, err := sourceReadMarkdown(body, nil)
	if err != nil {
		t.Fatal(err)
	}
	if markdown != want {
		t.Fatalf("markdown = %q, want %q", markdown, want)
	}
	document, err := sourceReadHTML(body, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{
		`<h2 class="txtar-member"><code>a.go</code></h2>`,
		`<p>package a</p>`,
		`<h2 class="txtar-member"><code>b.go</code></h2>`,
		`<p>package b</p>`,
	} {
		if !strings.Contains(document, fragment) {
			t.Errorf("HTML does not contain %q:\n%s", fragment, document)
		}
	}
}

func TestWriteSourceRead_JSONIncludesOrderedImages(t *testing.T) {
	body := imageSourceBody()
	var out bytes.Buffer
	if err := writeSourceRead(&out, body, globalOptions{jsonOutput: true}, nil); err != nil {
		t.Fatal(err)
	}
	var got sourceReadJSON
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Fragments) != 3 {
		t.Fatalf("fragments = %d, want 3", len(got.Fragments))
	}
	if got.Fragments[1].ImageURL != "https://example.test/image" || got.Fragments[1].ImageID != "image-1" {
		t.Errorf("image fragment = %+v", got.Fragments[1])
	}
	if got.Fragments[0].Text != "A" || got.Fragments[2].Text != "B" {
		t.Errorf("fragment order = %+v", got.Fragments)
	}
}

func TestSourceReadMarkdownFlagAfterCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	inv, err := parseInvocation([]string{"source", "read", "--markdown", "source-1"}, func(string) string { return "" }, &stdout, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if inv.name != "source read" || !inv.globals.sourceReadMarkdown || len(inv.args) != 1 || inv.args[0] != "source-1" {
		t.Fatalf("invocation = %+v", inv)
	}
}

func TestSourceReadHTMLFlagAfterCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	inv, err := parseInvocation([]string{"source", "read", "--html", "source-1"}, func(string) string { return "" }, &stdout, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if inv.name != "source read" || !inv.globals.sourceReadHTML || len(inv.args) != 1 || inv.args[0] != "source-1" {
		t.Fatalf("invocation = %+v", inv)
	}
}
