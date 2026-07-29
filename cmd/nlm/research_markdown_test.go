package main

import (
	"testing"

	"github.com/tmc/nlm/notebooklm"
)

func TestResearchMarkdown(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		report  string
		sources []notebooklm.ResearchSource
		want    string
	}{
		{
			name:   "multiple citations and unknown index",
			report: "First [cite: 2, 1, 2]. Unknown [cite: 9]. Mixed [cite: 1, 9].\n",
			sources: []notebooklm.ResearchSource{
				{CitationIndex: 1, Title: "One ] source", URL: "https://example.com/one_(x)"},
				{CitationIndex: 2, Title: "Two", URL: "https://example.com/two"},
			},
			want: "First [^2][^1]. Unknown [cite: 9]. Mixed [^1][cite: 9].\n\n" +
				"[^1]: [One \\] source](<https://example.com/one_(x)>)\n" +
				"[^2]: [Two](<https://example.com/two>)\n",
		},
		{
			name:   "fenced code is unchanged",
			report: "Before [cite: 1].\n\n```text\nliteral [cite: 1]\n```\n",
			sources: []notebooklm.ResearchSource{
				{CitationIndex: 1, Title: "Source", URL: "https://example.com"},
			},
			want: "Before [^1].\n\n```text\nliteral [cite: 1]\n```\n\n" +
				"[^1]: [Source](<https://example.com>)\n",
		},
		{
			name:   "no indexed sources",
			report: "Unchanged [cite: 1].",
			sources: []notebooklm.ResearchSource{
				{Title: "Source", URL: "https://example.com"},
			},
			want: "Unchanged [cite: 1].",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := researchMarkdown(tt.report, tt.sources); got != tt.want {
				t.Fatalf("researchMarkdown() =\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}
