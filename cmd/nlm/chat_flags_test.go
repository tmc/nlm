package main

import "testing"

func TestParseSourceSelectionArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantPos []string
		wantSel selectorOptions
		wantErr string
	}{
		{
			name:    "selectors without positional source ids",
			args:    []string{"nb", "--source-match", "^spec/"},
			wantPos: []string{"nb"},
			wantSel: selectorOptions{SourceMatch: "^spec/"},
		},
		{
			name:    "positional source ids still work",
			args:    []string{"nb", "src-1", "src-2"},
			wantPos: []string{"nb", "src-1", "src-2"},
		},
		{
			name:    "missing source ids and selectors",
			args:    []string{"nb"},
			wantErr: "missing source ids or selectors",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sourceIDsFlag, sourceMatchFlag = "", ""
			got, gotPos, err := parseSourceSelectionArgs(tt.args)
			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr {
					t.Fatalf("parseSourceSelectionArgs(%q) error = %v, want %q", tt.args, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseSourceSelectionArgs(%q) error = %v", tt.args, err)
			}
			if got.Selectors != tt.wantSel {
				t.Fatalf("parseSourceSelectionArgs(%q) selectors = %+v, want %+v", tt.args, got.Selectors, tt.wantSel)
			}
			if len(gotPos) != len(tt.wantPos) {
				t.Fatalf("parseSourceSelectionArgs(%q) positional = %q, want %q", tt.args, gotPos, tt.wantPos)
			}
			for i := range gotPos {
				if gotPos[i] != tt.wantPos[i] {
					t.Fatalf("parseSourceSelectionArgs(%q) positional = %q, want %q", tt.args, gotPos, tt.wantPos)
				}
			}
		})
	}
}

func TestParseGenerateChatArgs(t *testing.T) {
	showThinking, thinkingJSONL, verbose, citationMode = false, false, false, ""
	conversationID, useWebChat = "", false
	sourceIDsFlag, sourceMatchFlag = "", ""

	got, gotPos, err := parseGenerateChatArgs([]string{
		"nb",
		"why",
		"--conversation", "conv-1",
		"--thinking",
		"--source-match", "^spec/",
		"now",
	})
	if err != nil {
		t.Fatalf("parseGenerateChatArgs error = %v", err)
	}
	if got.ConversationID != "conv-1" || !got.Render.ShowThinking || got.Selectors.SourceMatch != "^spec/" {
		t.Fatalf("parseGenerateChatArgs opts = %+v", got)
	}
	wantPos := []string{"nb", "why", "now"}
	if len(gotPos) != len(wantPos) {
		t.Fatalf("parseGenerateChatArgs positional = %q, want %q", gotPos, wantPos)
	}
	for i := range gotPos {
		if gotPos[i] != wantPos[i] {
			t.Fatalf("parseGenerateChatArgs positional = %q, want %q", gotPos, wantPos)
		}
	}
}

func TestParseChatArgs(t *testing.T) {
	promptFile, citationMode = "", ""
	showChatHistory, showThinking, thinkingJSONL, verbose = false, false, false, false
	sourceIDsFlag, sourceMatchFlag = "", ""

	got, gotPos, err := parseChatArgs([]string{
		"nb",
		"--prompt-file", "prompt.txt",
		"--history",
		"--citations", "tail",
		"--source-ids", "a,b",
	})
	if err != nil {
		t.Fatalf("parseChatArgs error = %v", err)
	}
	if got.PromptFile != "prompt.txt" || !got.ShowHistory || got.Render.CitationMode != "tail" || got.Selectors.SourceIDs != "a,b" {
		t.Fatalf("parseChatArgs opts = %+v", got)
	}
	if len(gotPos) != 1 || gotPos[0] != "nb" {
		t.Fatalf("parseChatArgs positional = %q, want [nb]", gotPos)
	}
}

func TestParseGenerateReportArgs(t *testing.T) {
	reportPrompt, reportInstructions, citationMode = "", "", ""
	reportSections = 0
	showThinking, thinkingJSONL, verbose = false, false, false
	sourceIDsFlag, sourceMatchFlag = "", ""

	got, gotPos, err := parseGenerateReportArgs([]string{
		"nb",
		"--sections", "3",
		"--prompt", "# {topic}",
		"--source-match", "^guide/",
	})
	if err != nil {
		t.Fatalf("parseGenerateReportArgs error = %v", err)
	}
	if got.Sections != 3 || got.Prompt != "# {topic}" || got.Selectors.SourceMatch != "^guide/" {
		t.Fatalf("parseGenerateReportArgs opts = %+v", got)
	}
	if len(gotPos) != 1 || gotPos[0] != "nb" {
		t.Fatalf("parseGenerateReportArgs positional = %q, want [nb]", gotPos)
	}
}
