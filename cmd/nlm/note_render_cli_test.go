package main

import (
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
			command, ok := lookupCommand("note read")
			if !ok {
				t.Fatal("note read command not found")
			}
			parsed, err := parseCommandSpec(command.spec, command.surfaceSpec, test.args, globalOptions{})
			var got noteReadArgs
			if err == nil {
				got, err = decodeNoteReadArgs(parsed)
			}
			if (err != nil) != test.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, test.wantErr)
			}
			if test.wantErr {
				return
			}
			if got.Options.Format != test.wantFormat || got.Options.OutFile != test.wantOut || got.Options.Open != test.wantOpen {
				t.Errorf("options = %+v", got.Options)
			}
			if got.NotebookID != "nb" || got.NoteID != "note" {
				t.Errorf("arguments = %+v", got)
			}
		})
	}
}

func TestFormatNoteTextCompatibility(t *testing.T) {
	if got, want := formatNoteText("Title", "body\n"), "# Title\n\nbody\n\n"; got != want {
		t.Fatalf("text = %q, want %q", got, want)
	}
}
