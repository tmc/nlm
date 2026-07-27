package richrender

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/tmc/nlm/internal/notebooklm/api"
)

// decodeHTMLPayload extracts and decodes the JSON blob the page embeds in its
// <script id="chat-data"> block, so tests can assert on the structured model
// the client renders rather than scraping the runtime DOM.
func decodeHTMLPayload(t *testing.T, html string) htmlPayload {
	t.Helper()
	const open = `<script id="chat-data" type="application/json">`
	_, rest, found := strings.Cut(html, open)
	if !found {
		t.Fatalf("chat-data script block not found")
	}
	blob, _, found := strings.Cut(rest, "</script>")
	if !found {
		t.Fatalf("chat-data script block not closed")
	}
	var p htmlPayload
	if err := json.Unmarshal([]byte(blob), &p); err != nil {
		t.Fatalf("decode chat-data: %v\nblob: %q", err, blob)
	}
	return p
}

func renderToString(t *testing.T, doc ChatDocument, ctx RenderContext) string {
	t.Helper()
	var buf bytes.Buffer
	if err := renderChatHTML(&buf, doc, ctx); err != nil {
		t.Fatalf("renderChatHTML: %v", err)
	}
	return buf.String()
}

// answerBody extracts the server-rendered answer HTML for message msgIdx from the
// page (the content of its <template class="answer-body"> block), so tests can
// assert on the structural HTML Go now emits rather than scraping a runtime DOM.
func answerBody(t *testing.T, html string, msgIdx int) string {
	t.Helper()
	open := `<template class="answer-body" data-msg="` + strconv.Itoa(msgIdx) + `">`
	_, rest, found := strings.Cut(html, open)
	if !found {
		t.Fatalf("answer-body template for msg %d not found", msgIdx)
	}
	body, _, found := strings.Cut(rest, "</template>")
	if !found {
		t.Fatalf("answer-body template for msg %d not closed", msgIdx)
	}
	return body
}

func TestRenderChatHTMLSelfContained(t *testing.T) {
	doc := ChatDocument{
		Title: "Design review",
		Messages: []ChatMessage{
			{Role: "user", Content: "How does auth work?"},
			{
				Role:    "assistant",
				Content: "Auth uses stored cookies and a token.",
				Citations: []api.Citation{{
					SourceIndex: 1,
					SourceID:    "11111111-2222-3333-4444-555555555555",
					Title:       "auth.go",
					StartChar:   0,
					EndChar:     10,
					Confidence:  0.91,
					Excerpt:     "loadStoredEnv reads the cookie jar",
					SourceStart: 100,
					SourceEnd:   180,
				}},
			},
		},
	}
	html := renderToString(t, doc, RenderContext{})

	// No external requests: no http(s) src/href, no CDN.
	for _, bad := range []string{"http://", "https://", "src=\"//", "cdn."} {
		if strings.Contains(html, bad) {
			t.Errorf("page references external resource %q; must be self-contained", bad)
		}
	}
	// Inline CSS and JS present.
	if !strings.Contains(html, "<style>") || !strings.Contains(html, "<script>") {
		t.Errorf("expected inline <style> and <script>")
	}
	// The excerpt, a confidence value, and the source handle survive to the blob.
	p := decodeHTMLPayload(t, html)
	src := p.Messages[1].Markers[0].Sources[0]
	if src.Excerpt != "loadStoredEnv reads the cookie jar" {
		t.Errorf("excerpt = %q", src.Excerpt)
	}
	if !src.HasConf || src.Confidence != 0.91 {
		t.Errorf("confidence not carried: hasConf=%v conf=%v", src.HasConf, src.Confidence)
	}
	if src.Handle != "11111111" {
		t.Errorf("handle = %q, want 11111111", src.Handle)
	}
	if src.Title != "auth.go" {
		t.Errorf("title = %q", src.Title)
	}
}

func TestRenderChatHTMLMobileInteraction(t *testing.T) {
	doc := ChatDocument{
		Messages: []ChatMessage{{
			Role:    "assistant",
			Content: "A wide equation $$x_1 + x_2 + x_3 = y$$ [1]",
			Citations: []api.Citation{{
				SourceIndex: 1,
				SourceID:    "source-mobile",
				Title:       "Mobile source",
				StartChar:   0,
				EndChar:     15,
				Excerpt:     "A cited passage.",
			}},
		}},
	}
	page := renderToString(t, doc, RenderContext{})
	tests := []struct {
		name string
		want string
	}{
		{name: "touch detection", want: `window.matchMedia("(hover: none), (pointer: coarse)")`},
		{name: "tap preview", want: `function touchPreview(event, anchor, marker, key)`},
		{name: "tap pins", want: `pinnedAnchor = anchor;`},
		{name: "tap prevents jump", want: `event.preventDefault();`},
		{name: "tap away", want: `if (pinnedAnchor && !card.contains(event.target)) closeCard();`},
		{name: "close affordance", want: `close.setAttribute("aria-label", "Close citation preview")`},
		{name: "desktop hover retained", want: `a.addEventListener("mouseenter", function () { showCard(a, marker, key); });`},
		{name: "rail stacks", want: `.rail {
    position: static; top: auto; max-height: none; overflow: visible;`},
		{name: "phone card", want: `position: fixed; left: 10px !important; right: 10px; top: auto !important;`},
		{name: "touch target", want: `height: 44px; transform: translateY(-50%);`},
		{name: "math scroll", want: `grid-column: 2; min-width: 0; max-width: 100%; overflow-x: auto;`},
		{name: "page overflow guarded", want: `html, body { max-width: 100%; overflow-x: hidden; }`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if !strings.Contains(page, test.want) {
				t.Fatalf("mobile HTML missing %q", test.want)
			}
		})
	}
	if strings.Contains(page, `onclick=`) || strings.Contains(page, `ontouchstart=`) {
		t.Fatal("mobile wiring uses inline event handlers")
	}
}

// TestRenderChatHTMLMarkerLinking checks the citations-at-the-bottom model: the
// answer text carries the server's own [N] markers, so the client links those
// literal markers down to a numbered Citations section rather than deriving
// inline highlight spans from character offsets (which double-printed the marker
// and misaligned). The payload still carries the source data for each marker.
func TestRenderChatHTMLMarkerLinking(t *testing.T) {
	doc := ChatDocument{
		Messages: []ChatMessage{{
			Role:    "assistant",
			Content: "Alpha grounds this [1] and that [2].",
			Citations: []api.Citation{
				{SourceIndex: 1, SourceID: "abcdef0123456789", Title: "t1", StartChar: 0, EndChar: 5, Confidence: 0.9, Excerpt: "e1"},
				{SourceIndex: 2, SourceID: "99887766aabbccdd", Title: "t2", StartChar: 6, EndChar: 9, Confidence: 0.6, Excerpt: "e2"},
			},
		}},
	}
	html := renderToString(t, doc, RenderContext{})

	// Both markers' sources are in the payload.
	p := decodeHTMLPayload(t, html)
	if got := len(p.Messages[0].Markers); got != 2 {
		t.Fatalf("want 2 markers in payload, got %d", got)
	}

	// The server links the literal [N] markers to citation entries (server-rendered
	// answer body) and the client builds a bottom Citations section — not inline
	// .cite spans with superscripts.
	if strings.Contains(html, `el("span", "cite")`) {
		t.Errorf("obsolete inline-span construction still present")
	}
	// The answer body is now server-rendered HTML: each literal [N] we have a
	// citation for is an anchor down to its entry.
	if !strings.Contains(html, `<a class="citelink" href="#cite-0-1" data-msg="0" data-cite="1">1</a>`) {
		t.Errorf("server-rendered [1] link missing from answer body")
	}
	if !strings.Contains(html, `<a class="citelink" href="#cite-0-2" data-msg="0" data-cite="2">2</a>`) {
		t.Errorf("server-rendered [2] link missing from answer body")
	}
	// The client still builds the bottom Citations section, and its jump-target id
	// scheme must match the server's citelink hrefs.
	if !strings.Contains(html, `"cite-entry"`) || !strings.Contains(html, `"citations"`) {
		t.Errorf("bottom citations section construction missing from script")
	}
	if !strings.Contains(html, `"cite-" + msgIdx + "-" + idx`) {
		t.Errorf("citation id scheme missing from script")
	}
}

func TestRenderChatHTMLEscaping(t *testing.T) {
	doc := ChatDocument{
		Title: `Review <b>& "friends"</b>`,
		Messages: []ChatMessage{{
			Role:    "assistant",
			Content: `See <script>alert(1)</script> & more.`,
			Citations: []api.Citation{{
				SourceIndex: 1,
				SourceID:    "deadbeefcafef00d",
				Title:       `<img src=x onerror=alert(2)> & co`,
				StartChar:   0,
				EndChar:     3,
				Excerpt:     `a & b <hr> </script>\n1\tbad\n2\tonload=alert(3)`,
				Confidence:  0.5,
			}},
		}},
	}
	html := renderToString(t, doc, RenderContext{})

	// A raw executable <script>alert must never appear: the server content and
	// excerpt carry the literal text but only inside the JSON string, escaped.
	if strings.Contains(html, "<script>alert(1)</script>") {
		t.Errorf("unescaped server <script> leaked into markup")
	}
	if strings.Contains(html, "<img src=x onerror=alert(2)>") {
		t.Errorf("unescaped server <img> leaked into markup")
	}
	// The </script> literal inside the excerpt must be escaped so it cannot
	// close the data block early (json escapes < to <).
	if strings.Contains(html, `</script>`+"\n</head>") {
		// closing tag of our own real block is fine; ensure the excerpt's did
		// not create an early close by checking the payload still decodes.
	}
	p := decodeHTMLPayload(t, html)
	// If the excerpt's </script> had closed the block early, decode would have
	// failed or lost content. Assert the literal text round-trips intact.
	src := p.Messages[0].Markers[0].Sources[0]
	if src.Excerpt != "a & b <hr> </script>\n1\tbad\n2\tonload=alert(3)" {
		t.Errorf("excerpt round-trip = %q", src.Excerpt)
	}
	if src.Title != `<img src=x onerror=alert(2)> & co` {
		t.Errorf("title round-trip = %q", src.Title)
	}
	if p.Messages[0].Content != `See <script>alert(1)</script> & more.` {
		t.Errorf("content round-trip = %q", p.Messages[0].Content)
	}
	// The <title> element must carry an escaped title, not raw markup.
	if strings.Contains(html, `<title>Review <b>`) {
		t.Errorf("unescaped title in <title> element")
	}
	if !strings.Contains(html, "Review &lt;b&gt;") {
		t.Errorf("escaped title not found in <title> element")
	}
}

func TestRenderChatHTMLMultibyteNoCorruption(t *testing.T) {
	// Multibyte content and excerpt must round-trip with no replacement chars.
	// The answer is rendered verbatim (no offset slicing), so the risk is only in
	// the JSON blob encoding — assert content and excerpt survive byte-for-byte.
	content := "héllo 世界 🌍 end [1]"
	doc := ChatDocument{
		Messages: []ChatMessage{{
			Role:    "assistant",
			Content: content,
			Citations: []api.Citation{{
				SourceIndex: 1, SourceID: "feedface00000000", Title: "t",
				StartChar: 0, EndChar: 5, Excerpt: "héllo 世界 🌍", Confidence: 0.95,
			}},
		}},
	}
	html := renderToString(t, doc, RenderContext{})
	if strings.ContainsRune(html, '�') {
		t.Errorf("output contains U+FFFD replacement char; multibyte content corrupted")
	}
	p := decodeHTMLPayload(t, html)
	if p.Messages[0].Content != content {
		t.Errorf("content round-trip = %q, want %q", p.Messages[0].Content, content)
	}
	if !strings.Contains(p.Messages[0].Markers[0].Sources[0].Excerpt, "🌍") {
		t.Errorf("excerpt lost its emoji")
	}
}

// TestRenderChatHTMLPerSourceExcerpts checks that a multi-source marker carries
// every source's own excerpt (not just the first), so the card, rail, and bottom
// entry can each show one excerpt per source.
func TestRenderChatHTMLPerSourceExcerpts(t *testing.T) {
	doc := ChatDocument{
		Messages: []ChatMessage{{
			Role:    "assistant",
			Content: "Grounded in several sources [1].",
			Citations: []api.Citation{
				{SourceIndex: 1, SourceID: "aaaa0000aaaa0000", Title: "A", Excerpt: "excerpt from source A", Confidence: 0.9},
				{SourceIndex: 1, SourceID: "bbbb1111bbbb1111", Title: "B", Excerpt: "excerpt from source B", Confidence: 0.7},
			},
		}},
	}
	p := decodeHTMLPayload(t, renderToString(t, doc, RenderContext{}))
	srcs := p.Messages[0].Markers[0].Sources
	if len(srcs) != 2 {
		t.Fatalf("want 2 sources under the marker, got %d", len(srcs))
	}
	if srcs[0].Excerpt != "excerpt from source A" || srcs[1].Excerpt != "excerpt from source B" {
		t.Errorf("each source must keep its own excerpt: got %q and %q", srcs[0].Excerpt, srcs[1].Excerpt)
	}
}

func TestRenderChatHTMLHideConfidenceAndSpans(t *testing.T) {
	doc := ChatDocument{
		Messages: []ChatMessage{{
			Role:    "assistant",
			Content: "answer body here",
			Citations: []api.Citation{{
				SourceIndex: 1, SourceID: "aabbccddeeff0011", Title: "t",
				StartChar: 0, EndChar: 6, Confidence: 0.4,
				SourceStart: 10, SourceEnd: 40, Excerpt: "e",
			}},
		}},
	}
	html := renderToString(t, doc, RenderContext{HideConfidence: true, HideSpans: true})
	p := decodeHTMLPayload(t, html)
	src := p.Messages[0].Markers[0].Sources[0]
	if src.HasConf {
		t.Errorf("HideConfidence set but confidence present")
	}
	if src.Location != "" {
		t.Errorf("HideSpans set but source span present: %q", src.Location)
	}
}

func TestRenderChatHTMLWeakConfidence(t *testing.T) {
	doc := ChatDocument{
		Messages: []ChatMessage{{
			Role:    "assistant",
			Content: "weak and strong",
			Citations: []api.Citation{
				{SourceIndex: 1, SourceID: "s1s1s1s1s1s1s1s1", Title: "weak", StartChar: 0, EndChar: 4, Confidence: weakConfidence - 0.2},
				{SourceIndex: 2, SourceID: "s2s2s2s2s2s2s2s2", Title: "strong", StartChar: 9, EndChar: 15, Confidence: weakConfidence + 0.1},
			},
		}},
	}
	html := renderToString(t, doc, RenderContext{})
	p := decodeHTMLPayload(t, html)
	byTitle := map[string]htmlCitation{}
	for _, mk := range p.Messages[0].Markers {
		for _, s := range mk.Sources {
			byTitle[s.Title] = s
		}
	}
	if !byTitle["weak"].Weak {
		t.Errorf("below-threshold confidence should be weak")
	}
	if byTitle["strong"].Weak {
		t.Errorf("above-threshold confidence should not be weak")
	}
}

func TestRenderChatHTMLResolvedLocationAndTitle(t *testing.T) {
	// resolveTitle overrides the server title; loadSource-backed locations win
	// over the raw source span. Here we supply a resolveTitle only and confirm
	// it is used; location resolution requires a loader, so absent one we fall
	// back to the source span.
	doc := ChatDocument{
		Messages: []ChatMessage{{
			Role:    "assistant",
			Content: "resolved answer",
			Citations: []api.Citation{{
				SourceIndex: 1, SourceID: "res0res0res0res0", Title: "server-title",
				StartChar: 0, EndChar: 8, SourceStart: 200, SourceEnd: 260, Confidence: 0.8,
			}},
		}},
	}
	ctx := RenderContext{
		ResolveTitle: func(id string) string {
			if id == "res0res0res0res0" {
				return "notebook-title"
			}
			return ""
		},
	}
	html := renderToString(t, doc, ctx)
	p := decodeHTMLPayload(t, html)
	src := p.Messages[0].Markers[0].Sources[0]
	if src.Title != "notebook-title" {
		t.Errorf("resolveTitle not applied: %q", src.Title)
	}
	if src.Location != "src 200-260" {
		t.Errorf("source-span fallback = %q, want src 200-260", src.Location)
	}
}

// TestRenderChatHTMLPreservesExcerptNewlines pins F1 on the HTML surface: the
// excerpt box is white-space:pre-wrap and exists to show multi-line cited text.
// The newline must survive verbatim in the JSON payload (the client renders it
// via textContent, so pre-wrap displays the structure).
func TestRenderChatHTMLPreservesExcerptNewlines(t *testing.T) {
	excerpt := "line one\n  indented two\n\nline four"
	doc := ChatDocument{Messages: []ChatMessage{
		{Role: "assistant", Content: "See the config [1].", Citations: []api.Citation{
			{SourceIndex: 1, SourceID: "cfgabcd-1", Title: "config", Excerpt: excerpt, StartChar: 4, EndChar: 10, Confidence: 0.9},
		}},
	}}
	payload := decodeHTMLPayload(t, renderToString(t, doc, RenderContext{ExcerptBudget: 200}))
	var got string
	for _, m := range payload.Messages {
		for _, mk := range m.Markers {
			for _, s := range mk.Sources {
				if s.Excerpt != "" {
					got = s.Excerpt
				}
			}
		}
	}
	if !strings.Contains(got, "\n") {
		t.Fatalf("excerpt newline collapsed in payload: %q", got)
	}
	if got != excerpt {
		t.Errorf("excerpt not preserved verbatim:\n got  %q\n want %q", got, excerpt)
	}
}

func TestBuildCitationStructuredExcerpt(t *testing.T) {
	runs := []api.ExcerptRun{
		{Text: "plain "},
		{Text: "code", Code: true},
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
	citation := api.Citation{
		SourceIndex: 1,
		SourceID:    "source-1",
		Excerpt:     flat.String(),
		ExcerptRuns: runs,
	}
	got := buildCitation(citation, RenderContext{}, nil, 600)
	if got.Excerpt != flat.String() {
		t.Fatalf("flat excerpt = %q, want %q", got.Excerpt, flat.String())
	}
	if len(got.ExcerptRuns) != len(runs) {
		t.Fatalf("excerpt runs = %#v", got.ExcerptRuns)
	}
	if !got.ExcerptRuns[1].Code {
		t.Errorf("code run = %#v", got.ExcerptRuns[1])
	}
	if got.ExcerptRuns[2].Link != "https://example.test/docs" ||
		got.ExcerptRuns[3].Link != "mailto:test@example.test" {
		t.Errorf("safe links = %#v", got.ExcerptRuns[2:4])
	}
	if got.ExcerptRuns[4].Link != "" {
		t.Errorf("unsafe link survived sanitization: %#v", got.ExcerptRuns[4])
	}
	if got.ExcerptRuns[6].Code || got.ExcerptRuns[6].Link != "" {
		t.Errorf("unknown mark acquired a style: %#v", got.ExcerptRuns[6])
	}

	page := renderToString(t, ChatDocument{Messages: []ChatMessage{{
		Role: "assistant", Content: "claim [1]", Citations: []api.Citation{citation},
	}}}, RenderContext{})
	for _, want := range []string{
		`document.createTextNode(run.text)`,
		`document.createElement("a")`,
		`anchor.target = "_blank"`,
		`anchor.rel = "noopener noreferrer"`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("chat excerpt renderer missing %q", want)
		}
	}
	if strings.Contains(page, `innerHTML`) {
		t.Error("chat excerpt renderer must not use innerHTML")
	}
	if strings.Contains(page, `javascript:alert(1)`) {
		t.Error("unsafe excerpt href reached the HTML payload")
	}
	if strings.Contains(page, `</script><script>alert("text")</script>`) {
		t.Error("hostile excerpt text closed the JSON script")
	}
}

func TestSafeExcerptLink(t *testing.T) {
	tests := []struct {
		in   string
		want string
		ok   bool
	}{
		{in: "https://example.test/a", want: "https://example.test/a", ok: true},
		{in: "HTTP://example.test/a", want: "HTTP://example.test/a", ok: true},
		{in: "mailto:test@example.test", want: "mailto:test@example.test", ok: true},
		{in: "/relative/path", want: "/relative/path", ok: true},
		{in: "#anchor", want: "#anchor", ok: true},
		{in: " javascript:alert(1) "},
		{in: "java\tscript:alert(1)"},
		{in: "data:text/html,bad"},
		{in: "vbscript:bad"},
	}
	for _, test := range tests {
		t.Run(test.in, func(t *testing.T) {
			got, ok := safeExcerptLink(test.in)
			if got != test.want || ok != test.ok {
				t.Errorf("safeExcerptLink(%q) = %q, %v; want %q, %v", test.in, got, ok, test.want, test.ok)
			}
		})
	}
}

// TestRenderChatHTMLRailAndBottom pins the layout: an assistant turn with
// citations gets BOTH the sticky Sources rail (the at-a-glance index beside the
// answer) and the bottom Citations section (the full read), and the inline [N]
// markers become underlined links. All three surfaces must be present.
func TestRenderChatHTMLRailAndBottom(t *testing.T) {
	doc := ChatDocument{
		Messages: []ChatMessage{{
			Role:    "assistant",
			Content: "Backed by a source [1].",
			Citations: []api.Citation{
				{SourceIndex: 1, SourceID: "aaaa0000aaaa0000", Title: "A", Excerpt: "cited text A", Confidence: 0.9},
			},
		}},
	}
	html := renderToString(t, doc, RenderContext{})
	for _, want := range []string{
		`"assistant-grid"`, // two-column answer + rail
		`"rail"`,           // the right Sources bar
		`"ref"`,            // a rail entry
		`"ref-excerpt"`,    // the cited source text, underlined in the rail
		`"citations"`,      // the bottom section
		`"cite-entry"`,     // a bottom entry (jump target)
		`"citelink"`,       // the inline [N] link
	} {
		if !strings.Contains(html, want) {
			t.Errorf("layout element %s missing from page", want)
		}
	}
}

func TestRenderChatHTMLRailNavigation(t *testing.T) {
	doc := ChatDocument{Messages: []ChatMessage{{
		Role:    "assistant",
		Content: "Grounded claim. [1]",
		Citations: []api.Citation{{
			SourceIndex: 1,
			SourceID:    "source-1",
			StartChar:   0,
			EndChar:     15,
		}},
	}}}
	html := renderToString(t, doc, RenderContext{})
	for _, want := range []string{
		`var detail = el("a", "ref-action", "Details");`,
		`var passage = el("button", "ref-action", "Passage");`,
		`passage.type = "button";`,
		`function jumpToPassage(key)`,
		`var target = (groundEls[key] || [])[0];`,
		`target.scrollIntoView({ block: "center", behavior: "smooth" });`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("HTML missing citation navigation %q", want)
		}
	}
}

// buildGroundedDoc makes a one-assistant-turn doc whose answer carries an inline
// [1] marker, for exercising grounded-span validation. The answer is:
//
//	"Alpha beta gamma. [1] Delta."
//
// runes:  A=0 … the sentence "Alpha beta gamma." spans [0,17), " [1]" starts at
// 18, and the [1] token occupies runes [19,22).
const groundedAnswer = "Alpha beta gamma. [1] Delta."

func TestGroundedSpanValidatesAndCarries(t *testing.T) {
	doc := ChatDocument{Messages: []ChatMessage{{
		Role:    "assistant",
		Content: groundedAnswer,
		Citations: []api.Citation{
			// A real, in-range range covering "Alpha beta gamma." — must be carried.
			{SourceIndex: 1, SourceID: "s1", StartChar: 0, EndChar: 17, Confidence: 0.9},
		},
	}}}
	p := decodeHTMLPayload(t, renderToString(t, doc, RenderContext{}))
	m := p.Messages[0].Markers
	if len(m) != 1 || m[0].Span == nil {
		t.Fatalf("expected one marker with a span, got %+v", m)
	}
	if m[0].Span.Start != 0 || m[0].Span.End != 17 {
		t.Errorf("span = %+v, want {0,17}", m[0].Span)
	}
}

func TestGroundedSpanSkipsBadRanges(t *testing.T) {
	cases := []struct {
		name       string
		start, end int
	}{
		{"zero-width point", 20, 20},
		{"inverted", 17, 5},
		{"out of range", 0, 999},
		{"overlaps the [1] marker", 15, 22}, // [19,22) is the marker token
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := ChatDocument{Messages: []ChatMessage{{
				Role:      "assistant",
				Content:   groundedAnswer,
				Citations: []api.Citation{{SourceIndex: 1, SourceID: "s1", StartChar: tc.start, EndChar: tc.end}},
			}}}
			p := decodeHTMLPayload(t, renderToString(t, doc, RenderContext{}))
			if sp := p.Messages[0].Markers[0].Span; sp != nil {
				t.Errorf("%s: span should be nil (falls back to [N] underline), got %+v", tc.name, sp)
			}
		})
	}
}

// Wire answer offsets count UTF-16 code units, not runes: an astral-plane
// character (🔬, one rune, two UTF-16 units) shifts every offset after it by one
// unit. The grounded span is validated and carried in that UTF-16 space, then
// mapped to runes at the render seam — so the underline must land on the exact
// intended text, never split a codepoint or drift by the astral count.
func TestGroundedSpanMultibyte(t *testing.T) {
	// "🔬 Alpha beta gamma. [1]" — 🔬 is one rune (index 0) but TWO UTF-16 units.
	content := "🔬 Alpha beta gamma. [1]"
	// In UTF-16 units: 🔬=2, space=1, so "Alpha beta gamma." begins at unit 3 and
	// is 17 units long → the wire sends [3,20). (In rune space that same text is
	// [2,19); the render must recover exactly that.)
	doc := ChatDocument{Messages: []ChatMessage{{
		Role:      "assistant",
		Content:   content,
		Citations: []api.Citation{{SourceIndex: 1, SourceID: "s1", StartChar: 3, EndChar: 20}},
	}}}
	html := renderToString(t, doc, RenderContext{})
	p := decodeHTMLPayload(t, html)
	sp := p.Messages[0].Markers[0].Span
	// The span is kept in wire (UTF-16) space on the marker.
	if sp == nil || sp.Start != 3 || sp.End != 20 {
		t.Fatalf("span = %+v, want {3,20} (UTF-16 units)", sp)
	}
	// The [1] marker token is at UTF-16 units [22,25); the span [3,20) must not
	// overlap it.
	if rangeOverlapsAny(sp.Start, sp.End, markerTokenRangesUTF16(content)) {
		t.Errorf("valid span wrongly flagged as overlapping a marker")
	}
	// The rendered underline must cover exactly "Alpha beta gamma." — proof the
	// UTF-16→rune mapping placed it on the right characters, not one early.
	if want := ">Alpha beta gamma.<"; !strings.Contains(html, `class="grounded"`) || !strings.Contains(html, want) {
		t.Errorf("grounded underline did not wrap the intended text %q\nhtml: %s", want, html)
	}
}

func TestMarkerTokenRangesUTF16(t *testing.T) {
	// Emoji (2 UTF-16 units) + space (1) then "[12]" at UTF-16 units [3,7).
	got := markerTokenRangesUTF16("🔬 [12] tail")
	if len(got) != 1 || got[0] != [2]int{3, 7} {
		t.Errorf("markerTokenRangesUTF16 = %v, want [[3 7]]", got)
	}
}

// structuredAnswerDoc is a newline-free answer whose rich tree carries a heading
// paragraph, a body paragraph, a two-item list (a flush item then a nested one),
// a separator, and a closing paragraph. Because the server strips newlines from
// the structured form, the content is one run; the block offsets slice it back
// into the right pieces. Offsets (runes): "Heading"=[0,7), "Body text."=[7,17),
// "first"=[17,22), "second"=[22,28), "End."=[28,32).
func structuredAnswerDoc() ChatDocument {
	content := "HeadingBody text.firstsecondEnd."
	rich := &RichDocument{Blocks: []richSpan{
		{Start: "0", End: "7", Group: &richGroup{Children: []richSpan{
			{Start: "0", End: "7", Leaf: &richLeaf{Text: "Heading", Marks: &richMarks{Flag1: true}}},
		}}},
		{Start: "7", End: "17", Group: &richGroup{Children: []richSpan{
			{Start: "7", End: "17", Leaf: &richLeaf{Text: "Body text."}},
		}}},
		{Start: "17", End: "28", Group: &richGroup{Children: []richSpan{
			{Start: "17", End: "22", Group: &richGroup{ListItem: &richListItem{Nesting: 0}, Children: []richSpan{
				{Start: "17", End: "22", Leaf: &richLeaf{Text: "first"}},
			}}},
			{Start: "22", End: "28", Group: &richGroup{ListItem: &richListItem{Nesting: 1}, Children: []richSpan{
				{Start: "22", End: "28", Leaf: &richLeaf{Text: "second"}},
			}}},
		}}},
		{Start: "28", End: "28", Separator: true},
		{Start: "28", End: "32", Group: &richGroup{Children: []richSpan{
			{Start: "28", End: "32", Leaf: &richLeaf{Text: "End."}},
		}}},
	}}
	return ChatDocument{Messages: []ChatMessage{{Role: "assistant", Content: content, Rich: rich}}}
}

// TestRenderChatHTMLRichStructure pins the server-side tree render: a newline-free
// answer with a decoded rich tree becomes real <h4>/<p>/<ul><li>/<hr> structure,
// carrying the right text sliced from content by each block's offsets.
func TestRenderChatHTMLRichStructure(t *testing.T) {
	html := renderToString(t, structuredAnswerDoc(), RenderContext{})
	body := answerBody(t, html, 0)

	// A heading paragraph (flag1) becomes <h4>, not <p>; ordinary paragraphs stay
	// <p>; the list becomes <ul><li>; the nested item carries nest-1; a separator
	// becomes <hr>. Assert the exact structural HTML, in order.
	want := `<h4>Heading</h4><p>Body text.</p><ul><li>first</li><li class="nest-1">second</li></ul><hr><p>End.</p>`
	if body != want {
		t.Errorf("structured answer body:\n got  %q\n want %q", body, want)
	}
	// The flat-fallback wrapper must NOT be used when the tree applies.
	if strings.Contains(body, `answer-block`) {
		t.Errorf("structured answer wrongly rendered as a flat block: %q", body)
	}
}

// TestRenderChatHTMLFlatFallback pins the fallback: an answer whose content
// carries newlines (even with a rich tree present) renders as ONE flat block —
// no tree structure — so its literal [N] markers and any Markdown pass through
// unchanged. The markers still become links; the structure does not.
func TestRenderChatHTMLFlatFallback(t *testing.T) {
	doc := structuredAnswerDoc()
	// Give the content a newline: the tree render must not fire.
	doc.Messages[0].Content = "Line one.\n\n- a markdown bullet\n\nSee [1]."
	doc.Messages[0].Citations = []api.Citation{{
		SourceIndex: 1, SourceID: "aaaa0000aaaa0000", Title: "t", Excerpt: "e", Confidence: 0.9,
	}}
	html := renderToString(t, doc, RenderContext{})
	body := answerBody(t, html, 0)

	// One flat block, no structural tags.
	if !strings.HasPrefix(body, `<div class="answer-block">`) {
		t.Errorf("newline answer should be a flat block, got %q", body)
	}
	for _, tag := range []string{"<h4>", "<ul>", "<li", "<hr>"} {
		if strings.Contains(body, tag) {
			t.Errorf("flat fallback leaked structural tag %q: %q", tag, body)
		}
	}
	// The literal Markdown bullet survives verbatim (escaped text, not a <ul>).
	if !strings.Contains(body, "- a markdown bullet") {
		t.Errorf("markdown bullet not preserved verbatim: %q", body)
	}
	// The newline is preserved verbatim (the block is white-space:pre-wrap).
	if !strings.Contains(body, "Line one.\n") {
		t.Errorf("newline collapsed in flat answer: %q", body)
	}
	// The [1] marker still becomes a link.
	if !strings.Contains(body, `<a class="citelink" href="#cite-0-1" data-msg="0" data-cite="1">1</a>`) {
		t.Errorf("[1] link missing from flat answer: %q", body)
	}
}

// siblingListDoc models the REAL wire shape of a list: the items are TOP-LEVEL
// sibling blocks (each a group carrying its own ListItem), interleaved with an
// intro paragraph — NOT children of one list group. Offsets (runes):
// "Intro:"=[0,6), "first"=[6,11), "second"=[11,17), "nested"=[17,23).
func siblingListDoc() ChatDocument {
	content := "Intro:firstsecondnested"
	item := func(start, end, text string, nesting int) richSpan {
		return richSpan{Start: start, End: end, Group: &richGroup{
			ListItem: &richListItem{Nesting: nesting, Bullet: "•"},
			Children: []richSpan{{Start: start, End: end, Leaf: &richLeaf{Text: text}}},
		}}
	}
	rich := &RichDocument{Blocks: []richSpan{
		{Start: "0", End: "6", Group: &richGroup{Children: []richSpan{
			{Start: "0", End: "6", Leaf: &richLeaf{Text: "Intro:"}},
		}}},
		item("6", "11", "first", 0),
		item("11", "17", "second", 0),
		item("17", "23", "nested", 1),
	}}
	return ChatDocument{Messages: []ChatMessage{{Role: "assistant", Content: content, Rich: rich}}}
}

// TestRenderChatHTMLSiblingListCoalesce pins the coalesce pass: three list-item
// spans that are TOP-LEVEL SIBLINGS (the real wire shape, not children of one
// group) merge into ONE <ul> with three <li>, the nested item carrying nest-1.
// Without the pass each bullet would be its own <ul>.
func TestRenderChatHTMLSiblingListCoalesce(t *testing.T) {
	body := answerBody(t, renderToString(t, siblingListDoc(), RenderContext{}), 0)

	want := `<p>Intro:</p><ul><li>first</li><li>second</li><li class="nest-1">nested</li></ul>`
	if body != want {
		t.Errorf("sibling list did not coalesce into one <ul>:\n got  %q\n want %q", body, want)
	}
	// Exactly one <ul> — three separate <ul>s would mean the coalesce didn't fire.
	if n := strings.Count(body, "<ul>"); n != 1 {
		t.Errorf("got %d <ul> elements, want 1 (items must coalesce)", n)
	}
	if n := strings.Count(body, "<li"); n != 3 {
		t.Errorf("got %d <li> elements, want 3", n)
	}
}

// TestRenderChatHTMLXSSInAnswer verifies the escaping boundary end-to-end: an
// answer packed with breakout attempts (a </script> to close the data block, an
// <img onerror> to inject script, a ]]></script> CDATA-style break) renders as
// inert, escaped text in the answer body — never live markup — while a trailing
// [1] still links. Asserted on raw bytes (the escaped forms present, the live
// tags absent).
func TestRenderChatHTMLXSSInAnswer(t *testing.T) {
	payload := `</script><img src=x onerror=alert(1)> ]]></script> [1]`
	doc := ChatDocument{
		Messages: []ChatMessage{{
			Role:    "assistant",
			Content: payload,
			Citations: []api.Citation{{
				SourceIndex: 1, SourceID: "aaaa0000aaaa0000", Title: "t", Excerpt: "e", Confidence: 0.9,
			}},
		}},
	}
	html := renderToString(t, doc, RenderContext{})

	// No live injected markup anywhere on the page.
	for _, live := range []string{
		"<img src=x onerror=alert(1)>",
		"<img src=x onerror=alert(1)",
	} {
		if strings.Contains(html, live) {
			t.Errorf("live injected markup leaked into page: %q", live)
		}
	}
	// The answer body carries the payload as escaped, inert text.
	body := answerBody(t, html, 0)
	for _, esc := range []string{
		"&lt;/script&gt;",
		"&lt;img src=x onerror=alert(1)&gt;",
	} {
		if !strings.Contains(body, esc) {
			t.Errorf("expected escaped form %q in answer body, got %q", esc, body)
		}
	}
	// The only </script> tokens in the page are our own two real script blocks
	// (chat-data + the interactivity script); the payload's </script> did not
	// forge a third.
	if got := strings.Count(html, "</script>"); got != 2 {
		t.Errorf("unexpected </script> count %d (payload may have forged a close): want 2", got)
	}
	// The trailing [1] still links even amid the breakout text.
	if !strings.Contains(body, `<a class="citelink" href="#cite-0-1" data-msg="0" data-cite="1">1</a>`) {
		t.Errorf("[1] link missing from answer body: %q", body)
	}
}

// TestRenderChatHTMLMarkerRangeExpands checks that a range token [1-4] expands to
// one citelink per bound present, keeping the dash as literal text, while an
// index with no citation stays plain text.
func TestRenderChatHTMLMarkerRangeExpands(t *testing.T) {
	doc := ChatDocument{
		Messages: []ChatMessage{{
			Role:    "assistant",
			Content: "Grounded across [1-4] and also [2, 3] but not [9].",
			Citations: []api.Citation{
				{SourceIndex: 1, SourceID: "aaaa0000aaaa0001", Title: "a", Excerpt: "e", Confidence: 0.9},
				{SourceIndex: 2, SourceID: "aaaa0000aaaa0002", Title: "b", Excerpt: "e", Confidence: 0.9},
				{SourceIndex: 3, SourceID: "aaaa0000aaaa0003", Title: "c", Excerpt: "e", Confidence: 0.9},
				{SourceIndex: 4, SourceID: "aaaa0000aaaa0004", Title: "d", Excerpt: "e", Confidence: 0.9},
			},
		}},
	}
	body := answerBody(t, renderToString(t, doc, RenderContext{}), 0)

	// The range [1-4] drops its brackets, uses an en dash, and links both bounds.
	if !strings.Contains(body, `<sup class="citegroup"><a class="citelink" href="#cite-0-1" data-msg="0" data-cite="1">1</a>–<a class="citelink" href="#cite-0-4" data-msg="0" data-cite="4">4</a></sup>`) {
		t.Errorf("range [1-4] did not expand to linked bounds: %q", body)
	}
	// The list [2, 3] links each index with a compact comma.
	if !strings.Contains(body, `<sup class="citegroup"><a class="citelink" href="#cite-0-2" data-msg="0" data-cite="2">2</a>,<a class="citelink" href="#cite-0-3" data-msg="0" data-cite="3">3</a></sup>`) {
		t.Errorf("list [2, 3] did not link both indices: %q", body)
	}
	// [9] has no citation: it remains plain text inside the superscript.
	if strings.Contains(body, `href="#cite-0-9"`) {
		t.Errorf("[9] with no citation should not be a link: %q", body)
	}
	if !strings.Contains(body, `<sup class="citegroup">9</sup>`) {
		t.Errorf("[9] should survive as superscript text: %q", body)
	}
}

// TestRenderChatHTMLGroundedSpanInBody checks that a marker's validated grounded
// span becomes an underlined <span class="grounded"> in the server-rendered
// answer body, carrying the passage text and the data-* join keys.
func TestRenderChatHTMLGroundedSpanInBody(t *testing.T) {
	doc := ChatDocument{Messages: []ChatMessage{{
		Role:    "assistant",
		Content: groundedAnswer, // "Alpha beta gamma. [1] Delta."
		Citations: []api.Citation{
			{SourceIndex: 1, SourceID: "s1", StartChar: 0, EndChar: 17, Confidence: 0.9},
		},
	}}}
	body := answerBody(t, renderToString(t, doc, RenderContext{}), 0)
	if !strings.Contains(body, `<span class="grounded" data-msg="0" data-cite="1">Alpha beta gamma.</span>`) {
		t.Errorf("grounded span missing or wrong text: %q", body)
	}
	// The [1] token after it still links.
	if !strings.Contains(body, `<a class="citelink" href="#cite-0-1" data-msg="0" data-cite="1">1</a>`) {
		t.Errorf("[1] link missing after grounded span: %q", body)
	}
}

func TestAlignHTMLCitationsToVisibleMarkers(t *testing.T) {
	content := "Prefix decoration. Grounded answer text [4, 7]. More source content [7-9]."
	citations := []api.Citation{
		{SourceIndex: 1, SourceID: "source-4", StartChar: 5, EndChar: 25},
		{SourceIndex: 1, SourceID: "source-7", StartChar: 5, EndChar: 25},
		{SourceIndex: 2, SourceID: "source-7", StartChar: 26, EndChar: 45},
		{SourceIndex: 2, SourceID: "source-8", StartChar: 26, EndChar: 45},
		{SourceIndex: 2, SourceID: "source-9", StartChar: 26, EndChar: 45},
	}
	got := alignHTMLCitations(content, citations)
	wantIndices := []int{4, 7, 7, 8, 9}
	for i, want := range wantIndices {
		if got[i].SourceIndex != want {
			t.Errorf("citation %d index = %d, want %d", i, got[i].SourceIndex, want)
		}
	}
	runes := []rune(content)
	for _, test := range []struct {
		citation int
		want     string
	}{
		{citation: 0, want: "Grounded answer text"},
		{citation: 2, want: "More source content"},
	} {
		c := got[test.citation]
		if text := string(runes[c.StartChar:c.EndChar]); text != test.want {
			t.Errorf("citation %d text = %q, want %q", test.citation, text, test.want)
		}
	}

	markers := buildMarkers(ChatMessage{Content: content, Citations: citations}, RenderContext{}, 0)
	for _, marker := range markers {
		if marker.Index == 7 && len(marker.Spans) != 2 {
			t.Errorf("source 7 spans = %v, want two occurrences", marker.Spans)
		}
	}
}

func TestRenderChatHTMLLoadsMathJaxOnlyForMath(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{name: "plain", content: "plain prose", want: false},
		{name: "inline math", content: "equation $x^2$", want: true},
		{name: "display math", content: "$$E = mc^2$$", want: true},
		{name: "hostile math", content: "$<script>alert(1)</script>$", want: true},
		{name: "inline code", content: "run `$PATH:$HOME`", want: false},
		{name: "currency", content: "between $5 and $10 per unit", want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			doc := ChatDocument{Messages: []ChatMessage{{
				Role:    "assistant",
				Content: test.content,
			}}}
			got := renderToString(t, doc, RenderContext{})
			hasLoader := strings.Contains(got, `id="MathJax-script"`)
			hasConfig := strings.Contains(got, "window.MathJax")
			if hasLoader != test.want || hasConfig != test.want {
				t.Fatalf("MathJax loader/config = %v/%v, want %v", hasLoader, hasConfig, test.want)
			}
			if test.want {
				for _, want := range []string{
					`https://cdn.jsdelivr.net/npm/mathjax@3/es5/tex-mml-chtml.js`,
					`inlineMath: [['$', '$'], ['\\(', '\\)']]`,
					`displayMath: [['$$', '$$'], ['\\[', '\\]']]`,
				} {
					if !strings.Contains(got, want) {
						t.Errorf("HTML missing %q", want)
					}
				}
			}
			if test.name == "hostile math" {
				if strings.Contains(got, "$<script>alert(1)</script>$") {
					t.Fatal("math body was emitted as executable markup")
				}
				if !strings.Contains(got, "$&lt;script&gt;alert(1)&lt;/script&gt;$") {
					t.Fatal("escaped hostile math is missing")
				}
			}
		})
	}
}

// TestRenderChatHTMLMultibyteOffsets checks that the wire's UTF-16 answer
// offsets are mapped to runes before slicing: a leading astral emoji (two UTF-16
// units, one rune) must not shift the grounded slice. Feeding the offsets the
// wire actually sends, the rendered span text is still exactly the intended
// substring.
func TestRenderChatHTMLMultibyteOffsets(t *testing.T) {
	// "🔬 Alpha beta gamma. [1]" — 🔬 is one rune but TWO UTF-16 units. In UTF-16
	// units "Alpha beta gamma." is [3,20) and the [1] token is at [22,25).
	content := "🔬 Alpha beta gamma. [1]"
	doc := ChatDocument{Messages: []ChatMessage{{
		Role:      "assistant",
		Content:   content,
		Citations: []api.Citation{{SourceIndex: 1, SourceID: "s1", StartChar: 3, EndChar: 20, Confidence: 0.9}},
	}}}
	body := answerBody(t, renderToString(t, doc, RenderContext{}), 0)
	if !strings.Contains(body, `<span class="grounded" data-msg="0" data-cite="1">Alpha beta gamma.</span>`) {
		t.Errorf("multibyte grounded slice wrong (offset shifted?): %q", body)
	}
	// The emoji is preserved verbatim, before the span.
	if !strings.Contains(body, "🔬 <span") {
		t.Errorf("leading emoji not preserved before grounded span: %q", body)
	}
	if strings.ContainsRune(body, '�') {
		t.Errorf("replacement char in answer body; multibyte corrupted: %q", body)
	}
}

// thinkingBody extracts the server-rendered reasoning HTML for message msgIdx
// (the content of its <template class="thinking-body"> block), or "" when the
// message has no thinking template.
func thinkingBody(html string, msgIdx int) string {
	open := `<template class="thinking-body" data-msg="` + strconv.Itoa(msgIdx) + `">`
	_, rest, found := strings.Cut(html, open)
	if !found {
		return ""
	}
	body, _, _ := strings.Cut(rest, "</template>")
	return body
}

func thinkingDoc() ChatDocument {
	return ChatDocument{Messages: []ChatMessage{{
		Role:     "assistant",
		Content:  "The answer. [1]",
		Thinking: "First I consider X.\n\nThen I weigh Y.",
		Citations: []api.Citation{{
			SourceIndex: 1, SourceID: "aaaa0000aaaa0000", Title: "t", Excerpt: "e", Confidence: 0.9,
		}},
	}}}
}

// TestRenderChatHTMLThinkingOptIn pins the opt-in: the reasoning block is
// server-rendered into a <template class="thinking-body"> only when ShowThinking
// is set, and never otherwise. It carries the Reasoning label and the trace text
// with its newlines intact.
func TestRenderChatHTMLThinkingOptIn(t *testing.T) {
	// Off by default: no thinking template, and the trace is not in the page.
	off := renderToString(t, thinkingDoc(), RenderContext{})
	if b := thinkingBody(off, 0); b != "" {
		t.Errorf("thinking rendered without ShowThinking: %q", b)
	}
	if strings.Contains(off, "First I consider X") {
		t.Errorf("reasoning trace leaked into page without ShowThinking")
	}

	// On: the reasoning block is present with its label and text (newlines kept).
	on := renderToString(t, thinkingDoc(), RenderContext{ShowThinking: true})
	body := thinkingBody(on, 0)
	if body == "" {
		t.Fatal("no thinking-body template with ShowThinking set")
	}
	if !strings.Contains(body, `<span class="lbl">Reasoning</span>`) {
		t.Errorf("thinking block missing the Reasoning label: %q", body)
	}
	if !strings.Contains(body, "First I consider X.\n\nThen I weigh Y.") {
		t.Errorf("thinking trace or its newlines not preserved: %q", body)
	}
}

// TestRenderChatHTMLThinkingXSS verifies the reasoning block shares the answer
// body's escaping boundary: a hostile trace renders as inert escaped text, never
// live markup.
func TestRenderChatHTMLThinkingXSS(t *testing.T) {
	doc := thinkingDoc()
	doc.Messages[0].Thinking = `</template><script>alert(1)</script><img src=x onerror=alert(2)>`
	html := renderToString(t, doc, RenderContext{ShowThinking: true})

	// The data island plus the app script are the only two <script> elements; the
	// injected <script>alert and the <img onerror> must render as escaped text,
	// never a live tag. (The escaped forms &lt;script&gt; / &lt;img … contain the
	// substrings "alert(1)"/"onerror=" as inert text, so assert on the LIVE tag
	// forms, not the substrings.)
	if strings.Contains(html, "<script>alert(1)") {
		t.Errorf("thinking XSS: live <script>alert survived")
	}
	if strings.Contains(html, "<img") {
		t.Errorf("thinking XSS: a live <img tag survived (should be escaped &lt;img)")
	}
	if n := strings.Count(html, "<script"); n != 2 {
		t.Errorf("got %d <script> tags, want 2 (data island + app); injection may have added one", n)
	}
	// The trace must be present in escaped form (proof it rendered, inert).
	if !strings.Contains(html, "&lt;script&gt;alert(1)&lt;/script&gt;") {
		t.Errorf("thinking XSS payload not present in escaped form")
	}
}

// TestRenderChatHTMLReferencesUnderlined pins that inline [N] reference markers
// are underlined (dotted, distinct from the grounded passage's solid underline),
// so every reference on the page is underlined.
func TestRenderChatHTMLReferencesUnderlined(t *testing.T) {
	html := renderToString(t, thinkingDoc(), RenderContext{})
	// The citelink rule must declare an underline; a dotted style keeps it
	// distinct from .grounded.
	css, _, _ := strings.Cut(html, "</style>")
	link := cssRule(css, ".citelink {")
	if !strings.Contains(link, "text-decoration: underline") {
		t.Errorf(".citelink is not underlined: %q", link)
	}
	if !strings.Contains(link, "dotted") {
		t.Errorf(".citelink underline is not dotted (should differ from grounded): %q", link)
	}
}

// cssRule returns the text of the first CSS rule beginning with sel (up to its
// closing brace), or "" when not found.
func cssRule(css, sel string) string {
	_, rest, found := strings.Cut(css, sel)
	if !found {
		return ""
	}
	rule, _, _ := strings.Cut(rest, "}")
	return rule
}
