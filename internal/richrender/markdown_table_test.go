package richrender

import (
	"strings"
	"testing"
)

func TestMarkdownTable(t *testing.T) {
	for _, tt := range []struct {
		name, input string
		want        []string
		noTable     bool
	}{
		{"plain", "Name | Count\n--- | ---:\none | 2", []string{`<table>`, `<thead>`, `<th class="align-right">Count</th>`, `<td class="align-left">one</td>`}, false},
		{"outer pipes", "| Name | Count |\n| :---: | --- |\n| **one** | `2` |", []string{`<th class="align-center">Name</th>`, `<strong>one</strong>`, `<code>2</code>`}, false},
		{"escaped pipes", "| Name | Value |\n| --- | --- |\n| a\\|b | `c\\|d` |", []string{`>a|b</td>`, `<code>c|d</code>`}, false},
		{"short row", "a | b | c\n--- | --- | ---\nx | y", []string{`<td class="align-left"></td>`}, false},
		{"header mismatch", "a | b\n--- | --- | ---\nx | y", nil, true},
		{"prose", "alpha | beta\ngamma | delta", nil, true},
		{"code fence", "```\na | b\n--- | ---\nx | y\n```", []string{`<pre><code>a | b`}, true},
		{"escaped HTML", "a | b\n--- | ---\n<script> | <img onerror=alert(1)>", []string{`&lt;script&gt;`, `&lt;img onerror=alert(1)&gt;`}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			body := answerBodies(renderToString(t, ChatDocument{Messages: []ChatMessage{{Role: "assistant", Content: tt.input}}}, RenderContext{}))
			if strings.Contains(body, "<table>") == tt.noTable {
				t.Fatalf("table recognition: %s", body)
			}
			for _, want := range tt.want {
				if !strings.Contains(body, want) {
					t.Errorf("missing %q in %s", want, body)
				}
			}
		})
	}
}

func TestMarkdownTableGrounding(t *testing.T) {
	content := "Claim | Citation\n--- | ---\n**🌍 alpha** | [1,2]\n`beta\\|gamma` | [1]"
	span := func(text string) htmlSpan {
		start := strings.Index(content, text)
		return htmlSpan{Start: utf16Len(content[:start]), End: utf16Len(content[:start+len(text)])}
	}
	first := span("🌍 alpha")
	html, err := renderAnswerBody(0, ChatMessage{Role: "assistant", Content: content}, []htmlMarker{
		{Index: 1, Spans: []htmlSpan{first, span(`beta\|gamma`)}},
		{Index: 2, Spans: []htmlSpan{first}},
	})
	if err != nil {
		t.Fatal(err)
	}
	body := string(html)
	for _, want := range []string{`<table>`, `data-cites="1 2"`, `🌍 alpha</span>`, `href="#cite-0-1"`, `href="#cite-0-2"`, `>beta</span>`, `>|</span>`, `>gamma</span>`, `data-passages="1:2"`} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q in %s", want, body)
		}
	}
}
