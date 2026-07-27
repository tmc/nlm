package main

import (
	"strings"
	"testing"
)

func TestParseNoteReadArgs(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantFormat string
		wantOut    string
		wantOpen   bool
		wantErr    bool
	}{
		{"default", []string{"nb", "note"}, "text", "", false, false},
		{"markdown alias", []string{"--format=md", "nb", "note"}, "markdown", "", false, false},
		{"html destination", []string{"nb", "--format", "html", "note", "--out", "note.html", "--open"}, "html", "note.html", true, false},
		{"unknown format", []string{"--format", "pdf", "nb", "note"}, "", "", false, true},
		{"out with text", []string{"--out", "note.html", "nb", "note"}, "", "", false, true},
		{"open with markdown", []string{"--format", "markdown", "--open", "nb", "note"}, "", "", false, true},
		{"missing note", []string{"nb"}, "", "", false, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, positional, err := parseNoteReadArgs(test.args)
			if (err != nil) != test.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, test.wantErr)
			}
			if test.wantErr {
				return
			}
			if got.Format != test.wantFormat || got.OutFile != test.wantOut || got.Open != test.wantOpen {
				t.Errorf("options = %+v", got)
			}
			if strings.Join(positional, ",") != "nb,note" {
				t.Errorf("positional = %q", positional)
			}
		})
	}
}

func TestFormatNoteTextCompatibility(t *testing.T) {
	if got, want := formatNoteText("Title", "body\n"), "# Title\n\nbody\n\n"; got != want {
		t.Fatalf("text = %q, want %q", got, want)
	}
}
