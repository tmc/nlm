package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/notebooklm/api"
	"github.com/tmc/nlm/internal/richrender"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
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

func sourceReadProtoFixture() *pb.LoadSourceResponse {
	start, end := int64(10), int64(22)
	language := "go"
	return &pb.LoadSourceResponse{
		Source: &pb.Source{
			SourceId: &pb.SourceId{SourceId: "source-1"},
			Title:    "source.md",
			MediaData: &pb.SourceMediaData{
				Blob: &pb.SourceBlob{
					BlobRef:  "/contrib_service/blobrefs/notebooklm/source-1",
					MimeType: "text/markdown",
				},
			},
		},
		Content: &pb.LoadedSourceContent{
			Rows: &pb.LoadedSourceRows{
				Rows: []*pb.LoadedSourceRow{
					{
						Start: &start,
						End:   &end,
						CodeBlock: &pb.SpanCodeBlock{
							Code:     "package main\n",
							Language: &language,
						},
					},
					{
						Start: &start,
						End:   &end,
						Image: &pb.LoadedSourceImage{
							Url:     "https://example.test/image",
							ImageId: "image-1",
						},
					},
				},
			},
		},
	}
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

func TestWriteSourceReadJSONUsesStableModel(t *testing.T) {
	body := imageSourceBody()
	var out bytes.Buffer
	if err := writeSourceReadJSON(&out, body); err != nil {
		t.Fatal(err)
	}
	var got sourceReadJSON
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.SourceID != body.SourceID || got.Title != body.Title {
		t.Fatalf("document = %+v", got)
	}
	if len(got.Fragments) != len(body.Fragments) {
		t.Fatalf("fragments = %d, want %d", len(got.Fragments), len(body.Fragments))
	}
	if got.Fragments[1].ImageURL != body.Fragments[1].ImageURL ||
		got.Fragments[1].ImageID != body.Fragments[1].ImageID {
		t.Fatalf("image fragment = %+v", got.Fragments[1])
	}
}

func TestWriteSourceReadProtoJSONPreservesRowsAndBlob(t *testing.T) {
	response := sourceReadProtoFixture()
	var out bytes.Buffer
	if err := writeSourceReadProtoJSON(&out, response); err != nil {
		t.Fatal(err)
	}
	var got pb.LoadSourceResponse
	if err := protojson.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.GetSource().GetMediaData().GetBlob().GetBlobRef() != response.GetSource().GetMediaData().GetBlob().GetBlobRef() {
		t.Fatalf("media data = %+v", got.GetSource().GetMediaData())
	}
	rows := got.GetContent().GetRows().GetRows()
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	if rows[0].GetStart() != 10 || rows[0].GetEnd() != 22 || rows[0].GetCodeBlock().GetCode() != "package main\n" {
		t.Errorf("code row = %+v", rows[0])
	}
	if rows[1].GetImage().GetImageId() != "image-1" {
		t.Errorf("image row = %+v", rows[1])
	}
	var names map[string]any
	if err := json.Unmarshal(out.Bytes(), &names); err != nil {
		t.Fatal(err)
	}
	if _, ok := names["source"]; !ok {
		t.Fatalf("source field missing:\n%s", out.String())
	}
	if !strings.Contains(out.String(), `"code_block"`) || !strings.Contains(out.String(), `"blob_ref"`) {
		t.Fatalf("proto field names missing:\n%s", out.String())
	}
}

func TestWriteSourceReadProtoTextPreservesResponse(t *testing.T) {
	response := sourceReadProtoFixture()
	var out bytes.Buffer
	if err := writeSourceReadProtoText(&out, response); err != nil {
		t.Fatal(err)
	}
	if out.Len() == 0 {
		t.Fatal("prototext output is empty")
	}
	var got pb.LoadSourceResponse
	if err := prototext.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("parse prototext: %v\n%s", err, out.String())
	}
	if !proto.Equal(&got, response) {
		t.Fatalf("prototext round trip differs:\n%s", out.String())
	}
	for _, field := range []string{"code_block", "blob_ref"} {
		if !strings.Contains(out.String(), field+":") {
			t.Errorf("prototext does not contain %s field:\n%s", field, out.String())
		}
	}
}

type recordingSourceReadEmitter struct {
	items []richrender.ContentFragment
}

func (e *recordingSourceReadEmitter) EmitContent(item richrender.ContentFragment) error {
	e.items = append(e.items, item)
	return nil
}

func (*recordingSourceReadEmitter) FinishContent() error {
	return nil
}

func TestRenderSourceReadClassifiesFragmentsOnce(t *testing.T) {
	body := api.LoadSourceText{Fragments: []api.TextFragment{
		{Start: 0, End: 10, Text: "-- a.go --", Code: true},
		{Start: 10, End: 23, Text: "package main\n", Code: true},
		{Start: 23, End: 24, ImageURL: "https://example.test/image", ImageID: "image-1"},
		{Start: 24, End: 41, Text: "Figure 1: result"},
	}}
	var got recordingSourceReadEmitter
	if err := renderSourceRead(body, &got); err != nil {
		t.Fatal(err)
	}
	want := []richrender.ContentFragmentKind{
		richrender.ContentMember,
		richrender.ContentCode,
		richrender.ContentImage,
		richrender.ContentOrdinary,
	}
	if len(got.items) != len(want) {
		t.Fatalf("items = %d, want %d", len(got.items), len(want))
	}
	for i := range want {
		if got.items[i].Kind != want[i] {
			t.Errorf("item %d kind = %d, want %d", i, got.items[i].Kind, want[i])
		}
	}
	if got.items[0].MemberName != "a.go" {
		t.Errorf("member name = %q", got.items[0].MemberName)
	}
	if got.items[2].ImageAlt != "Figure 1: result" {
		t.Errorf("image alt = %q", got.items[2].ImageAlt)
	}
}

func TestSourceReadMarkdownFlagAfterCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	inv, err := parseInvocation([]string{"source", "read", "--markdown", "source-1"}, func(string) string { return "" }, &stdout, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if inv.name != "source read" || inv.globals.sourceReadMarkdown || !reflect.DeepEqual(inv.args, []string{"--markdown", "source-1"}) {
		t.Fatalf("invocation = %+v", inv)
	}
}

func TestSourceReadHTMLFlagAfterCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	inv, err := parseInvocation([]string{"source", "read", "--html", "source-1"}, func(string) string { return "" }, &stdout, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if inv.name != "source read" || inv.globals.sourceReadHTML || !reflect.DeepEqual(inv.args, []string{"--html", "source-1"}) {
		t.Fatalf("invocation = %+v", inv)
	}
}

func TestParseSourceReadArgsFormats(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		globals globalOptions
		format  string
	}{
		{name: "default", args: []string{"source-1"}, format: "text"},
		{name: "markdown", args: []string{"--format=markdown", "source-1"}, format: "markdown"},
		{name: "md", args: []string{"source-1", "--format", "md"}, format: "markdown"},
		{name: "html", args: []string{"--format", "html", "source-1"}, format: "html"},
		{name: "stable json", args: []string{"--format=json", "source-1"}, format: "json"},
		{name: "raw proto", args: []string{"--format=raw", "source-1"}, format: "raw"},
		{name: "proto text", args: []string{"--format=prototext", "source-1"}, format: "prototext"},
		{name: "raw json synonym", args: []string{"--format=raw-json", "source-1"}, format: "raw"},
		{name: "repeated format is last wins", args: []string{"--format=raw", "--format=prototext", "source-1"}, format: "prototext"},
		{name: "json alias", args: []string{"source-1"}, globals: globalOptions{jsonOutput: true}, format: "json"},
		{name: "markdown alias", args: []string{"source-1"}, globals: globalOptions{sourceReadMarkdown: true}, format: "markdown"},
		{name: "html alias", args: []string{"source-1"}, globals: globalOptions{sourceReadHTML: true}, format: "html"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			command, ok := lookupCommand("source read")
			if !ok {
				t.Fatal("source read command not found")
			}
			parsed, err := parseCommandSpec(command.spec, command.surfaceSpec, test.args, test.globals)
			if err != nil {
				t.Fatal(err)
			}
			got, err := decodeSourceReadArgs(parsed)
			if err != nil {
				t.Fatal(err)
			}
			if got.Options.sourceReadFormat != test.format {
				t.Errorf("format = %q, want %q", got.Options.sourceReadFormat, test.format)
			}
			if got.Target.SourceID != "source-1" || got.Target.NotebookID != "" || !got.Target.Grace {
				t.Errorf("arguments = %+v", got)
			}
		})
	}
}

func TestParseSourceReadArgsUsesNotebookFirst(t *testing.T) {
	command, ok := lookupCommand("source read")
	if !ok {
		t.Fatal("source read command not found")
	}
	parsed, err := parseCommandSpec(command.spec, command.surfaceSpec, []string{
		"--format=prototext",
		"notebook-1",
		"source-1",
	}, globalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	args, err := decodeSourceReadArgs(parsed)
	if err != nil {
		t.Fatal(err)
	}
	if args.Options.sourceReadFormat != "prototext" {
		t.Errorf("format = %q", args.Options.sourceReadFormat)
	}
	if got, want := strings.Join([]string{args.Target.NotebookID, args.Target.SourceID}, " "), "notebook-1 source-1"; got != want {
		t.Errorf("arguments = %q, want %q", got, want)
	}
	if !args.Target.Resolve || args.Target.Grace {
		t.Errorf("target = %+v", args.Target)
	}
}

func TestParseLegacySourceReadArgsKeepsChildFirstOrder(t *testing.T) {
	command, ok := lookupCommand("read-source")
	if !ok {
		t.Fatal("read-source command not found")
	}
	parsed, err := parseCommandSpec(command.spec, command.surfaceSpec, []string{
		"--format", "prototext",
		"source-1",
		"notebook-1",
	}, globalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	args, err := decodeSourceReadArgs(parsed)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join([]string{args.Target.SourceID, args.Target.NotebookID}, " "), "source-1 notebook-1"; got != want {
		t.Errorf("arguments = %q, want %q", got, want)
	}
	if args.Target.Resolve || args.Target.Grace {
		t.Errorf("legacy target = %+v", args.Target)
	}
	if args.Options.sourceReadFormat != "prototext" {
		t.Errorf("format = %q, want prototext", args.Options.sourceReadFormat)
	}
}

func TestSourceReadFormatAliasConflictsUnchanged(t *testing.T) {
	for _, format := range []string{"raw", "prototext"} {
		err := readSource(nil, "source-1", "", globalOptions{
			sourceReadFormat: format,
			jsonOutput:       true,
		})
		if err == nil || !strings.Contains(err.Error(), "use only one") {
			t.Errorf("%s readSource error = %v", format, err)
		}
	}
}

func TestSourceReadProtoTextMirrorsRawOutBehavior(t *testing.T) {
	command, ok := lookupCommand("source read")
	if !ok {
		t.Fatal("source read command not found")
	}
	for _, format := range []string{"raw", "prototext"} {
		_, err := parseCommandSpec(command.spec, command.surfaceSpec, []string{
			"--format=" + format,
			"--out",
			"source.txt",
			"source-1",
		}, globalOptions{})
		if !errors.Is(err, errBadArgs) {
			t.Errorf("%s --out error = %v, want invalid arguments", format, err)
		}
	}
}

func TestSourceReadUnknownFormatListsProtoText(t *testing.T) {
	opts := globalOptions{sourceReadFormat: "yaml"}
	err := normalizeSourceReadFormat(&opts)
	want := `unknown --format "yaml" (want text, markdown, html, json, raw, or prototext)`
	if err == nil || err.Error() != want {
		t.Fatalf("error = %v, want %q", err, want)
	}
}

func TestSourceReadHelpDocumentsProtoText(t *testing.T) {
	for _, path := range []string{"source read", "read-source"} {
		command, ok := lookupCommand(path)
		if !ok {
			t.Fatalf("%s command not found", path)
		}
		help := captureCommandStderr(t, func() {
			printCommandHelp(path, command)
		})
		for _, want := range []string{
			"text|markdown|html|json|raw|prototext",
			"The prototext format\nis the unstable LoadSource protobuf in protobuf text format.",
		} {
			if !strings.Contains(help, want) {
				t.Errorf("%s help does not contain %q:\n%s", path, want, help)
			}
		}
	}
}

func TestWarnDeprecatedSourceReadFormat(t *testing.T) {
	var out bytes.Buffer
	warnDeprecatedSourceReadFormat(&out, globalOptions{jsonOutput: true})
	if got, want := out.String(), "nlm: source read: --json is deprecated; use --format=json\n"; got != want {
		t.Errorf("warning = %q, want %q", got, want)
	}
}
