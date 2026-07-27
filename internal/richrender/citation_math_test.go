package richrender

import (
	"bytes"
	"strings"
	"testing"

	"github.com/tmc/nlm/internal/notebooklm/api"
)

func TestLiftTrailingMathCitation(t *testing.T) {
	tests := []struct {
		name      string
		token     string
		wantMath  string
		wantInner string
		wantOK    bool
	}{
		{
			name:      "display single",
			token:     `$$x_i = y^2 \quad [7]$$`,
			wantMath:  `$$x_i = y^2$$`,
			wantInner: "7",
			wantOK:    true,
		},
		{
			name:      "display list",
			token:     `$$\phi_i = z \quad [5, 6]$$`,
			wantMath:  `$$\phi_i = z$$`,
			wantInner: "5, 6",
			wantOK:    true,
		},
		{
			name:      "inline range",
			token:     `$x_i^2 \quad [23-25]$`,
			wantMath:  `$x_i^2$`,
			wantInner: "23-25",
			wantOK:    true,
		},
		{
			name:   "inline prose",
			token:  `$c/foo and marker \quad [7]$`,
			wantOK: false,
		},
		{
			name:   "missing quad",
			token:  `$$x_i = y^2 [7]$$`,
			wantOK: false,
		},
		{
			name:   "middle marker",
			token:  `$$x_i [7] = y^2$$`,
			wantOK: false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := liftTrailingMathCitation(test.token)
			if ok != test.wantOK {
				t.Fatalf("liftTrailingMathCitation(%q) ok = %v, want %v", test.token, ok, test.wantOK)
			}
			if !ok {
				return
			}
			if got.math != test.wantMath || got.inner != test.wantInner {
				t.Errorf("liftTrailingMathCitation(%q) = (%q, %q), want (%q, %q)",
					test.token, got.math, got.inner, test.wantMath, test.wantInner)
			}
		})
	}
}

func TestRenderChatHTMLLiftsMathCitations(t *testing.T) {
	content := "$$x_i = y^2 \\quad [7]$$\n" +
		"$$\\gamma_i = z^2 \\quad [7]$$\n" +
		"$a_i^2 \\quad [7]$"
	doc := chatDocument{Messages: []chatDocMessage{{
		Role:    "assistant",
		Content: content,
		Citations: []api.Citation{{
			SourceIndex: 7,
			SourceID:    "source-7",
		}},
	}}}
	got := renderToString(t, doc, chatRenderContext{})

	for _, want := range []string{
		`<span class="math-display-row"><span class="math-display-equation">$$x_i = y^2$$</span><span class="math-display-cite"><sup class="citegroup">`,
		`<span class="math-display-row"><span class="math-display-equation">$$\gamma_i = z^2$$</span><span class="math-display-cite"><sup class="citegroup">`,
		`$a_i^2$<sup class="citegroup"><a class="citelink" href="#cite-0-7" data-msg="0" data-cite="7">7</a></sup>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered chat missing %q:\n%s", want, got)
		}
	}
	bodyStart := strings.Index(got, `<template class="answer-body"`)
	if bodyStart < 0 {
		t.Fatalf("rendered chat has no answer template")
	}
	bodyEnd := strings.Index(got[bodyStart:], `</template>`)
	if bodyEnd < 0 {
		t.Fatalf("rendered chat has no answer template end")
	}
	body := got[bodyStart : bodyStart+bodyEnd]
	if strings.Contains(body, `\quad [7]`) {
		t.Errorf("citation remained inside rendered math:\n%s", body)
	}
	if got := strings.Count(body, `class="math-display-row"`); got != 2 {
		t.Errorf("display math row count = %d, want 2:\n%s", got, body)
	}
	for _, want := range []string{
		`grid-template-columns: minmax(0,1fr) auto minmax(0,1fr)`,
		`grid-column: 2; min-width: 0; max-width: 100%; overflow-x: auto`,
		`@media (max-width: 520px)`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("chat mobile math CSS missing %q", want)
		}
	}
}

func TestLiftSplitMathCitations(t *testing.T) {
	nodes := []answerNode{
		{Text: "$$"},
		{Tag: "span", Class: "grounded", DataMsg: "0", DataCite: "7", Text: `x_i = y^2 \quad`},
		{Text: " [7]$$\n$$"},
		{Tag: "span", Class: "grounded", DataMsg: "0", DataCite: "7", Text: `\gamma_i = z^2 \quad`},
		{Text: " [7]$$"},
	}
	byIndex := map[int]htmlMarker{7: {Index: 7}}
	nodes = liftSplitMathCitations(nodes, 0, byIndex)

	var out strings.Builder
	for _, node := range nodes {
		if err := renderAnswerNode(&out, node); err != nil {
			t.Fatal(err)
		}
	}
	got := out.String()
	for _, want := range []string{
		`<span class="math-display-row"><span class="math-display-equation"><span class="grounded" data-msg="0" data-cite="7">$$x_i = y^2$$</span></span><span class="math-display-cite"><sup class="citegroup"><a class="citelink"`,
		`<span class="math-display-row"><span class="math-display-equation"><span class="grounded" data-msg="0" data-cite="7">$$\gamma_i = z^2$$</span></span><span class="math-display-cite"><sup class="citegroup"><a class="citelink"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("split math render missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, `\quad`) {
		t.Errorf("split math retained quad citation spacing:\n%s", got)
	}
	if strings.Contains(got, `class="grounded" data-msg="0" data-cite="7"></span>`) {
		t.Errorf("split math left an empty grounded span:\n%s", got)
	}
}

func TestDisplayMathCitationPreservesGrounding(t *testing.T) {
	const content = `$$x_i = y^2 \quad [7]$$`
	nodes := inlineNodes(
		0,
		[]rune(content),
		0,
		len([]rune(content)),
		content,
		[]htmlMarker{{
			Index: 7,
			Spans: []htmlSpan{{Start: 2, End: 13}},
		}},
		map[int]htmlMarker{7: {Index: 7}},
	)
	var out strings.Builder
	for _, node := range nodes {
		if err := renderAnswerNode(&out, node); err != nil {
			t.Fatal(err)
		}
	}
	got := out.String()
	for _, want := range []string{
		`<span class="math-display-row">`,
		`<span class="math-display-equation"><span class="grounded" data-msg="0" data-cite="7">$$x_i = y^2$$</span></span>`,
		`<span class="math-display-cite"><sup class="citegroup">`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("display math render missing %q:\n%s", want, got)
		}
	}
}

func TestRenderNoteHTMLLiftsMathCitations(t *testing.T) {
	doc := noteDocument{
		Title: "Fabricated math",
		Flat:  `$$\phi_i = z^2 \quad [5, 6]$$`,
		Rich: &richDocument{Blocks: []richSpan{{
			Group: &richGroup{Children: []richSpan{{
				Leaf: &richLeaf{Text: `$$\phi_i = z^2 \quad [5, 6]$$`},
			}}},
		}}},
		Citations: []api.Citation{
			{SourceIndex: 5, SourceID: "source-5"},
			{SourceIndex: 6, SourceID: "source-6"},
		},
	}
	var out bytes.Buffer
	if err := renderNoteHTML(&out, doc); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, `<span class="math-display-row"><span class="math-display-equation"><span class="math-display">$$\phi_i = z^2$$</span></span><span class="math-display-cite"><sup class="citegroup">`) {
		t.Errorf("note math was not cleaned:\n%s", got)
	}
	for _, index := range []string{"5", "6"} {
		want := `data-cite="` + index + `">` + index + `</a>`
		if !strings.Contains(got, want) {
			t.Errorf("note citation %s was not linked:\n%s", index, got)
		}
	}
	if strings.Contains(got, `\quad [5, 6]`) {
		t.Errorf("citation remained inside note math:\n%s", got)
	}
	for _, want := range []string{
		`.math-display-equation{grid-column:2;min-width:0;max-width:100%;overflow-x:auto`,
		`@media(max-width:520px){.math-display-row{grid-template-columns:minmax(0,1fr) auto}`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("note mobile math CSS missing %q", want)
		}
	}
}
