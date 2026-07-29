package main

import (
	"testing"

	"github.com/tmc/nlm/notebooklm"
)

func TestParseAppCreateArgsWithOptions(t *testing.T) {
	t.Parallel()

	parsed := parseCreateCommandForTest(t, "app create", []string{
		"--type", "mindmap",
		"--instructions", "focus on architecture",
		"--source-ids", "src-1,src-2",
		"nb-1",
	})
	args, err := decodeAppCreateArgs(parsed, "")
	if err != nil {
		t.Fatalf("decodeAppCreateArgs: %v", err)
	}
	if args.Options.Type != "mindmap" || args.Options.Instructions != "focus on architecture" {
		t.Fatalf("type/instructions = %q/%q", args.Options.Type, args.Options.Instructions)
	}
	if args.Options.Selectors.SourceIDs != "src-1,src-2" {
		t.Fatalf("source ids = %q, want src-1,src-2", args.Options.Selectors.SourceIDs)
	}
	if args.NotebookID != "nb-1" {
		t.Fatalf("notebook id = %q, want nb-1", args.NotebookID)
	}
}

func TestParseAppCreateArgsUsesPositionalInstructions(t *testing.T) {
	t.Parallel()

	parsed := parseCreateCommandForTest(t, "app create", []string{
		"--type", "prototype",
		"nb-1",
		"build", "a", "study", "app",
	})
	args, err := decodeAppCreateArgs(parsed, "")
	if err != nil {
		t.Fatalf("decodeAppCreateArgs: %v", err)
	}
	if args.Options.Instructions != "build a study app" {
		t.Fatalf("instructions = %q, want positional join", args.Options.Instructions)
	}
	if args.NotebookID != "nb-1" {
		t.Fatalf("notebook id = %q, want nb-1", args.NotebookID)
	}
}

func TestDecodeMindmapCreateArgsType(t *testing.T) {
	t.Parallel()

	parsed := parseCreateCommandForTest(t, "mindmap create", []string{"nb-1", "map the sources"})
	args, err := decodeAppCreateArgs(parsed, "mindmap")
	if err != nil {
		t.Fatal(err)
	}
	if args.Options.Type != "mindmap" {
		t.Fatalf("type = %q, want mindmap", args.Options.Type)
	}

	parsed = parseCreateCommandForTest(t, "mindmap create", []string{
		"nb-1", "build an app", "--type", "prototype",
	})
	args, err = decodeAppCreateArgs(parsed, "mindmap")
	if err != nil {
		t.Fatal(err)
	}
	if args.Options.Type != "prototype" {
		t.Fatalf("type = %q, want prototype", args.Options.Type)
	}
}

func TestParseSlideDeckFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in      string
		want    notebooklm.SlideDeckFormat
		wantErr bool
	}{
		{"", notebooklm.SlideDeckFormatDetailed, false},
		{"detailed", notebooklm.SlideDeckFormatDetailed, false},
		{"DETAILED", notebooklm.SlideDeckFormatDetailed, false},
		{"detail", notebooklm.SlideDeckFormatDetailed, false},
		{"presenter", notebooklm.SlideDeckFormatPresenter, false},
		{" Presenter ", notebooklm.SlideDeckFormatPresenter, false},
		{"sparse", notebooklm.SlideDeckFormatPresenter, false},
		{"bogus", 0, true},
	}
	for _, tt := range tests {
		got, err := parseSlideDeckFormat(tt.in)
		if tt.wantErr {
			if err == nil {
				t.Errorf("parseSlideDeckFormat(%q) = %v, want error", tt.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseSlideDeckFormat(%q): %v", tt.in, err)
			continue
		}
		if got != tt.want {
			t.Errorf("parseSlideDeckFormat(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestParseSlidesCreateArgs(t *testing.T) {
	t.Parallel()

	// Flags before and after the positional notebook id; instructions optional.
	parsed := parseCreateCommandForTest(t, "deck create", []string{
		"--format", "presenter",
		"nb-1",
		"focus", "on", "the", "results",
		"--source-match", "^spec/",
	})
	args, err := decodeSlidesCreateArgs(parsed)
	if err != nil {
		t.Fatalf("decodeSlidesCreateArgs: %v", err)
	}
	if args.Options.Format != "presenter" {
		t.Fatalf("format = %q, want presenter", args.Options.Format)
	}
	if args.Options.Selectors.SourceMatch != "^spec/" {
		t.Fatalf("source-match = %q, want ^spec/", args.Options.Selectors.SourceMatch)
	}
	if args.NotebookID != "nb-1" || args.Instructions != "focus on the results" {
		t.Fatalf("arguments = %+v, want nb-1 and instructions", args)
	}

	// Notebook id alone (no instructions, no format) is valid.
	parsed = parseCreateCommandForTest(t, "deck create", []string{"nb-1"})
	if _, err := decodeSlidesCreateArgs(parsed); err != nil {
		t.Fatalf("decodeSlidesCreateArgs(nb only): %v", err)
	}

	// Missing notebook id is an error.
	if err := parseCreateCommandErrorForTest(t, "deck create", []string{"--format", "detailed"}); err == nil {
		t.Fatal("parseCommandSpec with no notebook id: want error")
	}

	// Invalid format is rejected at parse time.
	if err := parseCreateCommandErrorForTest(t, "deck create", []string{"--format", "bogus", "nb-1"}); err == nil {
		t.Fatal("parseCommandSpec with bad format: want error")
	}
}

func TestParseAudioVideoOptions(t *testing.T) {
	t.Parallel()

	parsed := parseCreateCommandForTest(t, "audio create", []string{
		"--length", "long",
		"--language", "es",
		"--audio-type", "debate",
		"nb-1",
		"compare the sources",
	})
	audioArgs, err := decodeAudioCreateArgs(parsed)
	if err != nil {
		t.Fatalf("decodeAudioCreateArgs: %v", err)
	}
	if audioArgs.Options.Length != "long" || audioArgs.Options.Language != "es" || audioArgs.Options.AudioType != "debate" {
		t.Fatalf("audio opts = %+v", audioArgs.Options)
	}
	if audioArgs.NotebookID != "nb-1" || audioArgs.Instructions != "compare the sources" {
		t.Fatalf("audio arguments = %+v", audioArgs)
	}

	parsed = parseCreateCommandForTest(t, "video create", []string{
		"--style", "whiteboard",
		"--language", "fr",
		"nb-1",
		"explain visually",
	})
	videoArgs, err := decodeVideoCreateArgs(parsed)
	if err != nil {
		t.Fatalf("decodeVideoCreateArgs: %v", err)
	}
	if videoArgs.Options.Style != "whiteboard" || videoArgs.Options.Language != "fr" {
		t.Fatalf("video opts = %+v", videoArgs.Options)
	}
	if videoArgs.NotebookID != "nb-1" || videoArgs.Instructions != "explain visually" {
		t.Fatalf("video arguments = %+v", videoArgs)
	}
}

func parseCreateCommandForTest(t *testing.T, path string, values []string) parsedCommand {
	t.Helper()
	command, ok := lookupCommand(path)
	if !ok {
		t.Fatalf("%s command not found", path)
	}
	parsed, err := parseCommandSpec(command.spec, command.surfaceSpec, values, globalOptions{})
	if err != nil {
		t.Fatalf("parseCommandSpec(%s): %v", path, err)
	}
	return parsed
}

func parseCreateCommandErrorForTest(t *testing.T, path string, values []string) error {
	t.Helper()
	command, ok := lookupCommand(path)
	if !ok {
		t.Fatalf("%s command not found", path)
	}
	_, err := parseCommandSpec(command.spec, command.surfaceSpec, values, globalOptions{})
	return err
}
