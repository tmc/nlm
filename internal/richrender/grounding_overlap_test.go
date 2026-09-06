package richrender

import (
	"strings"
	"testing"

	"github.com/tmc/nlm/notebooklm"
)

func TestChatGroundingOverlap(t *testing.T) {
	for _, markdown := range []bool{false, true} {
		content := "alpha beta gamma [1,2]"
		shift := 0
		if markdown {
			content = "**alpha beta gamma** [1,2]"
			shift = 2
		}
		doc := ChatDocument{Messages: []ChatMessage{{
			Role: "assistant", Content: content,
			Citations: []notebooklm.Citation{
				{SourceIndex: 1, SourceID: "one", StartChar: shift, EndChar: shift + 10},
				{SourceIndex: 2, SourceID: "two", StartChar: shift + 6, EndChar: shift + 16},
			},
		}}}
		body := answerBodies(renderToString(t, doc, RenderContext{}))
		for _, want := range []string{
			`data-cite="1">alpha </span>`,
			`data-cite="1" data-cites="1 2">beta</span>`,
			`data-cite="2"> gamma</span>`,
			`href="#cite-0-1"`, `href="#cite-0-2"`,
		} {
			if !strings.Contains(body, want) {
				t.Errorf("markdown=%v: missing %q in %s", markdown, want, body)
			}
		}
	}
}

func TestGroundedTextSharedRanges(t *testing.T) {
	runes := []rune("🌍 shared plain tail")
	markers := []htmlMarker{
		{Index: 2, Spans: []htmlSpan{{0, 8}, {0, 8}}},
		{Index: 1, Span: &htmlSpan{0, 8}},
		{Index: 3, Span: &htmlSpan{15, 19}},
	}
	got := groundedTextNodes(0, runes, 0, len(runes), markers)
	if len(got) != 3 || got[0].DataCites != "1 2" || got[0].Text != "🌍 shared" || got[1].Text != " plain " || got[1].DataCite != "" || got[2].DataCite != "3" {
		t.Fatalf("nodes = %#v", got)
	}
}

func TestGroundingOccurrenceAcrossFormatting(t *testing.T) {
	content := "**First claim** [1]. Later claim [1]."
	doc := ChatDocument{Messages: []ChatMessage{{
		Role: "assistant", Content: content,
		Citations: []notebooklm.Citation{
			{SourceIndex: 1, SourceID: "one", StartChar: 2, EndChar: 13},
			{SourceIndex: 1, SourceID: "one", StartChar: 21, EndChar: 32},
		},
	}}}
	body := answerBodies(renderToString(t, doc, RenderContext{}))
	for _, want := range []string{`data-passages="1:1">First claim</span>`, `data-passages="1:2">Later claim</span>`} {
		if !strings.Contains(body, want) {
			t.Errorf("missing occurrence %q in %s", want, body)
		}
	}
}

func TestRepeatedCitationSourcePassages(t *testing.T) {
	first := notebooklm.Citation{SourceIndex: 1, SourceID: "source", Excerpt: "first excerpt", SourceStart: 10, SourceEnd: 23}
	second := first
	second.Excerpt, second.SourceStart, second.SourceEnd = "second excerpt", 40, 54
	doc := ChatDocument{Messages: []ChatMessage{{Role: "assistant", Content: "Claim [1]", Citations: []notebooklm.Citation{first, second, first}}}}
	payload := decodeHTMLPayload(t, renderToString(t, doc, RenderContext{}))
	sources := payload.Messages[0].Markers[0].Sources
	if len(sources) != 2 || sources[0].Excerpt != first.Excerpt || sources[1].Excerpt != second.Excerpt {
		t.Fatalf("source passages = %#v", sources)
	}
}
