package richrender

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/notebooklm"
	"golang.org/x/net/html"
	"google.golang.org/protobuf/proto"
)

func TestNoteDocumentProjection(t *testing.T) {
	note := richNoteFixture()
	doc := noteDocumentFromAPI(note)
	if doc.Title != "Rich <Note>" {
		t.Fatalf("Title = %q", doc.Title)
	}
	if doc.Rich == nil || len(doc.Rich.Blocks) != 4 {
		t.Fatalf("Rich = %#v, want four blocks", doc.Rich)
	}
	if got := len(doc.Citations); got != 2 {
		t.Fatalf("Citations = %d, want 2", got)
	}
	for i, citation := range doc.Citations {
		if citation.SourceIndex != 1 {
			t.Errorf("citation %d index = %d, want 1", i, citation.SourceIndex)
		}
		if citation.Excerpt == "" {
			t.Errorf("citation %d has no excerpt", i)
		}
	}
}

func TestRenderNoteMarkdownRich(t *testing.T) {
	var out bytes.Buffer
	if err := renderNoteMarkdown(&out, noteDocumentFromAPI(richNoteFixture())); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{
		"# Rich <Note>",
		"## *Heading*",
		"`$x+y$`",
		"- first",
		"  - second",
		"#### Citations",
		"| 1 |",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("markdown missing %q:\n%s", want, got)
		}
	}
}

func TestRenderNoteHTMLRichAndEscaped(t *testing.T) {
	var out bytes.Buffer
	if err := renderNoteHTML(&out, noteDocumentFromAPI(richNoteFixture())); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if _, err := html.Parse(strings.NewReader(got)); err != nil {
		t.Fatalf("html round trip: %v", err)
	}
	for _, want := range []string{
		`<h4><em>Head<span class="grounded" data-msg="0" data-cite="1">ing</span></em></h4>`,
		"<ul><li>first</li><li class=\"nest-1\">second</li></ul>",
		"Equation $x+y$ cites",
		`href="#cite-0-1"`,
		`id="cite-0-1"`,
		`class="note-rail"`,
		`class="note-action note-detail" href="#cite-0-1" data-cite="1">Details</a>`,
		`class="note-action note-passage" type="button" data-cite="1">Passage</button>`,
		`document.querySelector('.grounded[data-cite="' + button.dataset.cite + '"]')`,
		`target.scrollIntoView({block: "center", behavior: "smooth"})`,
		`id="MathJax-script"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("html missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, `<code class="math">`) {
		t.Fatal("math was emitted inside code")
	}
	if strings.Contains(got, "<script>alert(1)</script>") {
		t.Fatal("server text was emitted as executable markup")
	}
	if !strings.Contains(got, "&lt;script&gt;alert(1)&lt;/script&gt;") {
		t.Fatal("escaped server text is missing")
	}
	if strings.Contains(got, `<img src=x onerror=alert(2)>`) {
		t.Fatal("event handler was emitted as executable markup")
	}
	if !strings.Contains(got, `&lt;img src=x onerror=alert(2)&gt; ]]&gt;`) {
		t.Fatal("escaped hostile server text is missing")
	}
	if strings.Count(got, `id="cite-0-1"`) != 1 {
		t.Fatalf("citation marker id count = %d, want 1", strings.Count(got, `id="cite-0-1"`))
	}

	out.Reset()
	unsafeLink := NoteDocument{
		Rich: &RichDocument{Blocks: []richSpan{{
			Group: &richGroup{Children: []richSpan{{
				Leaf: &richLeaf{
					Text:  "unsafe link",
					Marks: &richMarks{Link: "javascript:alert(3)"},
				},
			}}},
		}}},
	}
	if err := renderNoteHTML(&out, unsafeLink); err != nil {
		t.Fatal(err)
	}
	got = out.String()
	if _, err := html.Parse(strings.NewReader(got)); err != nil {
		t.Fatalf("hostile html round trip: %v", err)
	}
	if strings.Contains(got, "javascript:") {
		t.Fatal("unsafe URI was emitted")
	}
	if !strings.Contains(got, "unsafe link") {
		t.Fatal("unsafe link text is missing")
	}
}

func TestRenderNoteHTMLMobileInteraction(t *testing.T) {
	var out bytes.Buffer
	if err := renderNoteHTML(&out, noteDocumentFromAPI(richNoteFixture())); err != nil {
		t.Fatal(err)
	}
	page := out.String()
	tests := []struct {
		name string
		want string
	}{
		{name: "citation data", want: `<script id="note-data" type="application/json">`},
		{name: "touch detection", want: `window.matchMedia("(hover: none), (pointer: coarse)")`},
		{name: "tap preview", want: `function touchPreview(event, anchor, marker)`},
		{name: "tap pins", want: `pinnedAnchor = anchor;`},
		{name: "tap away", want: `if (pinnedAnchor && !card.contains(event.target)) closeCard();`},
		{name: "close affordance", want: `close.setAttribute("aria-label", "Close citation preview")`},
		{name: "desktop hover", want: `target.addEventListener("mouseenter", function () { showCard(target, marker); });`},
		{name: "rail stacks", want: `.note-rail{position:static;max-height:none;padding-top:1rem;border-top:1px solid #dadce0}`},
		{name: "phone card", want: `.note-card{position:fixed;left:10px!important;right:10px;top:auto!important;`},
		{name: "touch target", want: `height:44px;transform:translateY(-50%)`},
		{name: "math scroll", want: `.math-display-equation{grid-column:2;min-width:0;max-width:100%;overflow-x:auto;`},
		{name: "page overflow guarded", want: `body{font:16px/1.55 system-ui,sans-serif;max-width:76rem;margin:3rem auto;padding:0 1.25rem;color:#202124;overflow-x:hidden}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if !strings.Contains(page, test.want) {
				t.Fatalf("mobile note HTML missing %q", test.want)
			}
		})
	}
	if strings.Contains(page, `onclick=`) || strings.Contains(page, `ontouchstart=`) {
		t.Fatal("mobile note wiring uses inline event handlers")
	}
}

func TestRenderNoteHTMLCitationDataEscaped(t *testing.T) {
	doc := noteDocumentFromAPI(richNoteFixture())
	const excerpt = `</script><script>alert("citation")</script>`
	doc.Citations[0].Excerpt = excerpt
	var out bytes.Buffer
	if err := renderNoteHTML(&out, doc); err != nil {
		t.Fatal(err)
	}
	page := out.String()
	if strings.Contains(page, excerpt) {
		t.Fatal("citation data escaped its JSON script block")
	}
	const open = `<script id="note-data" type="application/json">`
	_, rest, ok := strings.Cut(page, open)
	if !ok {
		t.Fatal("note citation data not found")
	}
	blob, _, ok := strings.Cut(rest, "</script>")
	if !ok {
		t.Fatal("note citation data is not closed")
	}
	var markers []htmlMarker
	if err := json.Unmarshal([]byte(blob), &markers); err != nil {
		t.Fatal(err)
	}
	if got := markers[0].Sources[0].Excerpt; got != excerpt {
		t.Fatalf("excerpt = %q, want %q", got, excerpt)
	}
}

func TestRenderNoteHTMLStructuredExcerpt(t *testing.T) {
	runs := []notebooklm.ExcerptRun{
		{Text: "plain "},
		{Text: "code<&", Code: true},
		{Text: " http", Link: "https://example.test/docs"},
		{Text: " mail", Link: "mailto:test@example.test"},
		{Text: " bad", Link: "javascript:alert(1)"},
		{Text: ` </script><script>alert("text")</script>`},
		{Text: " unknown", RawMarks: []interface{}{nil, true}},
	}
	var flat strings.Builder
	for _, run := range runs {
		flat.WriteString(run.Text)
	}
	doc := NoteDocument{
		Title: "Structured excerpt",
		Flat:  "claim [1]",
		Citations: []notebooklm.Citation{{
			SourceIndex: 1,
			SourceID:    "source-1",
			Excerpt:     flat.String(),
			ExcerptRuns: runs,
		}},
	}
	var out bytes.Buffer
	if err := renderNoteHTML(&out, doc); err != nil {
		t.Fatal(err)
	}
	page := out.String()
	for _, want := range []string{
		`<code>code&lt;&amp;</code>`,
		`<a href="https://example.test/docs" target="_blank" rel="noopener noreferrer"> http</a>`,
		`<a href="mailto:test@example.test" target="_blank" rel="noopener noreferrer"> mail</a>`,
		`document.createTextNode(run.text)`,
		`document.createElement("a")`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("note structured excerpt missing %q", want)
		}
	}
	if strings.Contains(page, `javascript:alert(1)`) {
		t.Error("unsafe excerpt href reached the note page")
	}
	if strings.Contains(page, `</script><script>alert("text")</script>`) {
		t.Error("hostile excerpt text escaped its text context")
	}
	if strings.Contains(page, `innerHTML`) {
		t.Error("note excerpt renderer must not use innerHTML")
	}
}

func TestNoteCompoundMarkerGrounding(t *testing.T) {
	note := richNoteFixture()
	note.Note.ContentText = strings.Replace(note.Note.GetContentText(), "[1]", "[1-2]", 1)
	doc := noteDocumentFromAPI(note)
	if got, want := len(doc.Citations), 2; got != want {
		t.Fatalf("citations = %d, want %d", got, want)
	}
	for i, citation := range doc.Citations {
		if citation.SourceIndex != i+1 {
			t.Errorf("citation %d index = %d, want %d", i, citation.SourceIndex, i+1)
		}
	}
	var out bytes.Buffer
	if err := renderNoteHTML(&out, doc); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{
		`href="#cite-0-1"`,
		`href="#cite-0-2"`,
		`id="cite-0-1"`,
		`id="cite-0-2"`,
		`class="grounded"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("HTML missing %q", want)
		}
	}
}

func TestRenderNoteTextCitations(t *testing.T) {
	var out bytes.Buffer
	if err := renderNoteText(&out, noteDocumentFromAPI(richNoteFixture())); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{"# Rich <Note>", "Equation", "Citations", "source-a", "source-b"} {
		if !strings.Contains(got, want) {
			t.Errorf("text missing %q:\n%s", want, got)
		}
	}
}

func TestNotePageHasMath(t *testing.T) {
	tests := []struct {
		name string
		html string
		want bool
	}{
		{name: "inline dollars", html: "<p>Inline $a_i + b_j$ math.</p>", want: true},
		{name: "display dollars", html: "<p>$$E = mc^2$$</p>", want: true},
		{name: "inline parens", html: `<p>\(x_1\)</p>`, want: true},
		{name: "display brackets", html: `<p>\[x_1\]</p>`, want: true},
		{name: "no math", html: "<p>plain prose</p>", want: false},
		{name: "dollars only in code", html: `<pre><code>echo "$a and $b"</code></pre>`, want: false},
		{name: "dollars only in inline code", html: `<p>run <code>$PATH:$HOME</code> now</p>`, want: false},
		{name: "math beside code", html: `<p>$x^2$ and <code>$HOME</code></p>`, want: true},
		{name: "currency amounts", html: "<p>cost over $3 billion; consumers paid $1000 and later $100.</p>", want: false},
		{name: "currency pair mid-sentence", html: "<p>between $5 and $10 per unit</p>", want: false},
		{name: "math at end of sentence", html: "<p>bounded by $\\rho(Dg) < 1$.</p>", want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := notePageHasMath(test.html); got != test.want {
				t.Fatalf("notePageHasMath(%q) = %v, want %v", test.html, got, test.want)
			}
		})
	}
}

func TestRenderNoteHTMLLoadsMathJaxOnlyForMath(t *testing.T) {
	tests := []struct {
		name string
		flat string
		want bool
	}{
		{name: "plain", flat: "plain prose", want: false},
		{name: "inline math", flat: "equation $x^2$", want: true},
		{name: "display math", flat: "$$E = mc^2$$", want: true},
		{name: "inline code", flat: "`$PATH:$HOME`", want: false},
		{name: "currency", flat: "between $5 and $10 per unit", want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var out bytes.Buffer
			if err := renderNoteHTML(&out, NoteDocument{Title: "Test", Flat: test.flat}); err != nil {
				t.Fatal(err)
			}
			got := out.String()
			hasLoader := strings.Contains(got, `id="MathJax-script"`)
			hasConfig := strings.Contains(got, "window.MathJax")
			if hasLoader != test.want || hasConfig != test.want {
				t.Fatalf("MathJax loader/config = %v/%v, want %v:\n%s", hasLoader, hasConfig, test.want, got)
			}
		})
	}
}

func TestRenderNoteHTMLMathEscaping(t *testing.T) {
	doc := NoteDocument{
		Title: "Math",
		Flat:  "$$E = mc^2$$\n\n$<script>alert(1)</script>$",
	}
	var out bytes.Buffer
	if err := renderNoteHTML(&out, doc); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if _, err := html.Parse(strings.NewReader(got)); err != nil {
		t.Fatalf("html round trip: %v", err)
	}
	for _, want := range []string{
		`<span class="math-display">$$E = mc^2$$</span>`,
		"$&lt;script&gt;alert(1)&lt;/script&gt;$",
		`<script id="MathJax-script" async src="https://cdn.jsdelivr.net/npm/mathjax@3/es5/tex-mml-chtml.js"></script>`,
		`inlineMath: [['$', '$'], ['\\(', '\\)']]`,
		`displayMath: [['$$', '$$'], ['\\[', '\\]']]`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("html missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, `<script>alert(1)</script>`) {
		t.Fatal("math body was emitted as executable markup")
	}
	if strings.Contains(got, `<code class="math">`) {
		t.Fatal("math was emitted inside code")
	}
}

func TestRenderNoteHTMLPlainMarkdownSubset(t *testing.T) {
	doc := NoteDocument{
		Title: "Plain",
		Flat:  "## Head\n\n- **bold**\n- `code`\n\nMath $x$ <img src=x>",
	}
	var out bytes.Buffer
	if err := renderNoteHTML(&out, doc); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{
		"<h4>Head</h4>",
		"<ul><li><strong>bold</strong></li><li><code>code</code></li></ul>",
		"Math $x$ &lt;img src=x&gt;",
		"&lt;img src=x&gt;",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("html missing %q:\n%s", want, got)
		}
	}
}

func TestRenderNoteHTMLSuperscriptCitations(t *testing.T) {
	doc := NoteDocument{
		Title: "Citations",
		Flat:  "Range [1-4] and list [1, 2].",
		Citations: []notebooklm.Citation{
			{SourceIndex: 1, SourceID: "source-1"},
			{SourceIndex: 2, SourceID: "source-2"},
			{SourceIndex: 4, SourceID: "source-4"},
		},
	}
	var out bytes.Buffer
	if err := renderNoteHTML(&out, doc); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{
		`<sup class="citegroup"><a class="citelink" href="#cite-0-1" data-msg="0" data-cite="1">1</a>–<a class="citelink" href="#cite-0-4" data-msg="0" data-cite="4">4</a></sup>`,
		`<sup class="citegroup"><a class="citelink" href="#cite-0-1" data-msg="0" data-cite="1">1</a>,<a class="citelink" href="#cite-0-2" data-msg="0" data-cite="2">2</a></sup>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("html missing %q:\n%s", want, got)
		}
	}
}

func TestRenderNoteHTMLGroundedSpans(t *testing.T) {
	doc := NoteDocument{
		Title: "Grounding",
		Flat:  "Alpha claim [1]. Beta claim [1].",
		Citations: []notebooklm.Citation{
			{SourceIndex: 1, SourceID: "source-1", StartChar: 0, EndChar: 11},
			{SourceIndex: 1, SourceID: "source-2", StartChar: 17, EndChar: 27},
		},
	}
	markers := noteCitationMarkers(doc)
	if len(markers) != 1 || len(markers[0].Spans) != 2 {
		t.Fatalf("markers = %+v, want one marker with two spans", markers)
	}
	var out bytes.Buffer
	if err := renderNoteHTML(&out, doc); err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(out.String(), `class="grounded"`); got != 2 {
		t.Fatalf("grounded spans = %d, want 2", got)
	}
}

func richNoteFixture() *notebooklm.Note {
	span := func(start, end int64, text string, marks *pb.TextMarks) *pb.Span {
		return &pb.Span{
			Start: proto.Int64(start),
			End:   proto.Int64(end),
			Content: &pb.SpanContent{Value: &pb.SpanContent_Leaf{Leaf: &pb.TextLeaf{
				Text: proto.String(text), Marks: marks,
			}}},
		}
	}
	group := func(start, end int64, children ...*pb.Span) *pb.Span {
		elements := make([]*pb.SpanElement, 0, len(children))
		for _, child := range children {
			elements = append(elements, &pb.SpanElement{Value: &pb.SpanElement_Span{Span: child}})
		}
		return &pb.Span{
			Start: proto.Int64(start),
			End:   proto.Int64(end),
			Content: &pb.SpanContent{Value: &pb.SpanContent_Group{Group: &pb.SpanGroup{
				Spans: elements,
			}}},
		}
	}
	list := func(start, end int64, text string, nesting int64) *pb.Span {
		block := group(start, end, span(start, end, text, nil))
		block.GetContent().GetGroup().ListItem = &pb.ListItem{Nesting: proto.Int64(nesting)}
		return block
	}
	source := func(id string) *pb.SourceIdList { return &pb.SourceIdList{SourceId: id} }
	offset := func(start, end int64) *pb.OffsetRange {
		return &pb.OffsetRange{Start: proto.Int64(start), End: proto.Int64(end)}
	}
	grounding := func(id, excerpt string) *pb.Grounding {
		return &pb.Grounding{
			Score:      proto.Float64(0.75),
			ReplySpans: []*pb.OffsetRange{offset(8, 13)},
			SourceSpans: &pb.SpanList{Spans: []*pb.Span{
				span(100, 100+int64(len(excerpt)), excerpt, nil),
			}},
			SourceId: source(id),
		}
	}

	doc := &pb.RichDocument{Body: &pb.SpanLayers{
		Blocks: []*pb.Span{
			group(0, 7, span(0, 7, "Heading", &pb.TextMarks{Flag1: proto.Bool(true)})),
			group(7, 47, span(7, 47, "Equation $x+y$ cites <script>alert(1)</script>. <img src=x onerror=alert(2)> ]]>", nil)),
			list(47, 52, "first", 0),
			list(52, 58, "second", 1),
		},
		Annotations: []*pb.SourceAnnotation{
			{Source: source("source-a"), Range: offset(8, 13)},
			{Source: source("source-b"), Range: offset(8, 13)},
		},
	}}
	first := grounding("source-a", "first excerpt")
	second := grounding("source-b", "second excerpt")
	doc.Grounding = []*pb.GroundingRecord{
		{Source: source("source-a"), Grounding: first},
		{Source: source("source-b"), Grounding: second},
	}
	return &notebooklm.Note{
		Note: &pb.Note{
			NoteId: "note-1",
			Title:  "Rich <Note>",
			ContentText: "## *Heading*\n\nEquation $x+y$ cites [1] <script>alert(1)</script>. <img src=x onerror=alert(2)> ]]>\n\n" +
				"- first\n  - second",
		},
		Rich:      doc,
		Grounding: []*pb.Grounding{first, second},
	}
}
