package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestNoteCreateNamedContentInputs(t *testing.T) {
	devNull, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer devNull.Close()
	restoreStdin := replaceNoteStdin(t, devNull)
	defer restoreStdin()

	contentPath := filepath.Join(t.TempDir(), "content.md")
	if err := os.WriteFile(contentPath, []byte("file body\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name      string
		args      []string
		stdin     string
		wantBody  string
		wantGrace bool
	}{
		{
			name:     "content flag",
			args:     []string{"notebook-1", "Title", "--content", "flag body"},
			wantBody: "flag body",
		},
		{
			name:     "content file",
			args:     []string{"notebook-1", "Title", "--content-file", contentPath},
			wantBody: "file body\n",
		},
		{
			name:     "explicit empty content",
			args:     []string{"notebook-1", "Title", "--content="},
			wantBody: "",
		},
		{
			name:     "empty note",
			args:     []string{"notebook-1", "Title"},
			wantBody: "",
		},
		{
			name:      "positional grace",
			args:      []string{"notebook-1", "Title", "old body"},
			wantBody:  "old body",
			wantGrace: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			parsed := parseNoteCommand(t, "note create", test.args)
			args, err := decodeNoteCreateArgsWithStdin(parsed, false)
			if err != nil {
				t.Fatal(err)
			}
			if args.Grace != test.wantGrace {
				t.Errorf("Grace = %v, want %v", args.Grace, test.wantGrace)
			}
			var body string
			if err := runNoteCreate(&bytes.Buffer{}, strings.NewReader(test.stdin), args, func(_ string, content string) error {
				body = content
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			if body != test.wantBody {
				t.Errorf("content = %q, want %q", body, test.wantBody)
			}
		})
	}
}

func TestNoteCreateReadsImplicitAndExplicitStdin(t *testing.T) {
	tests := []struct {
		name string
		args []string
		body string
	}{
		{name: "implicit", args: []string{"notebook-1", "Title"}, body: "implicit body\n"},
		{name: "content file dash", args: []string{"notebook-1", "Title", "--content-file=-"}, body: "explicit body\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reader, writer, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			if _, err := writer.WriteString(test.body); err != nil {
				t.Fatal(err)
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			defer reader.Close()
			restoreStdin := replaceNoteStdin(t, reader)
			defer restoreStdin()

			parsed := parseNoteCommand(t, "note create", test.args)
			args, err := decodeNoteCreateArgs(parsed)
			if err != nil {
				t.Fatal(err)
			}
			var body string
			if err := runNoteCreate(&bytes.Buffer{}, reader, args, func(_ string, content string) error {
				body = content
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			if body != test.body {
				t.Errorf("content = %q, want %q", body, test.body)
			}
		})
	}
}

func TestNoteUpdateNamedMutations(t *testing.T) {
	devNull, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer devNull.Close()
	restoreStdin := replaceNoteStdin(t, devNull)
	defer restoreStdin()

	contentPath := filepath.Join(t.TempDir(), "content.md")
	if err := os.WriteFile(contentPath, []byte("file update\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name        string
		input       []string
		wantTitle   *string
		wantContent *string
	}{
		{
			name:      "title only",
			input:     []string{"notebook-1", "note-1", "--title", "New title"},
			wantTitle: noteString("New title"),
		},
		{
			name:        "content only",
			input:       []string{"notebook-1", "note-1", "--content", "New body"},
			wantContent: noteString("New body"),
		},
		{
			name:        "both",
			input:       []string{"notebook-1", "note-1", "--title", "New title", "--content", "New body"},
			wantTitle:   noteString("New title"),
			wantContent: noteString("New body"),
		},
		{
			name:        "empty content clears",
			input:       []string{"notebook-1", "note-1", "--content="},
			wantContent: noteString(""),
		},
		{
			name:        "content file",
			input:       []string{"notebook-1", "note-1", "--content-file", contentPath},
			wantContent: noteString("file update\n"),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			parsed := parseNoteCommand(t, "note update", test.input)
			args, err := decodeNoteUpdateArgsWithStdin(parsed, false)
			if err != nil {
				t.Fatal(err)
			}
			var gotTitle, gotContent *string
			if err := runNoteUpdate(
				&bytes.Buffer{},
				strings.NewReader(""),
				args,
				func(_, _ string) error {
					return errors.New("unexpected full update")
				},
				func(title, content *string) error {
					gotTitle, gotContent = title, content
					return nil
				},
			); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(gotTitle, test.wantTitle) || !reflect.DeepEqual(gotContent, test.wantContent) {
				t.Errorf("mutation = title %v, content %v; want title %v, content %v",
					stringValue(gotTitle), stringValue(gotContent),
					stringValue(test.wantTitle), stringValue(test.wantContent))
			}
		})
	}
}

func TestNoteUpdateReadsPipedStdin(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.WriteString("piped body\n"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	restoreStdin := replaceNoteStdin(t, reader)
	defer restoreStdin()

	parsed := parseNoteCommand(t, "note update", []string{"notebook-1", "note-1"})
	args, err := decodeNoteUpdateArgs(parsed)
	if err != nil {
		t.Fatal(err)
	}
	var gotContent *string
	if err := runNoteUpdate(
		&bytes.Buffer{},
		reader,
		args,
		func(_, _ string) error { return errors.New("unexpected full update") },
		func(_ *string, content *string) error {
			gotContent = content
			return nil
		},
	); err != nil {
		t.Fatal(err)
	}
	if gotContent == nil || *gotContent != "piped body\n" {
		t.Errorf("content = %v, want piped body", stringValue(gotContent))
	}
}

func TestNoteMutationUsageErrors(t *testing.T) {
	devNull, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer devNull.Close()
	restoreStdin := replaceNoteStdin(t, devNull)
	defer restoreStdin()

	tests := []struct {
		name  string
		path  string
		input []string
		usage string
	}{
		{
			name:  "create content sources conflict",
			path:  "note create",
			input: []string{"notebook-1", "Title", "--content", "body", "--content-file", "body.md"},
			usage: "usage: nlm note create <notebook-id> <title> [--content TEXT | --content-file FILE]\n",
		},
		{
			name:  "update empty title",
			path:  "note update",
			input: []string{"notebook-1", "note-1", "--title="},
			usage: "usage: nlm note update <notebook-id> <note-id> [--title TITLE] [--content TEXT | --content-file FILE]\n",
		},
		{
			name:  "update no mutation",
			path:  "note update",
			input: []string{"notebook-1", "note-1"},
			usage: "usage: nlm note update <notebook-id> <note-id> [--title TITLE] [--content TEXT | --content-file FILE]\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cmd, ok := lookupCommand(test.path)
			if !ok {
				t.Fatalf("%s not found", test.path)
			}
			var gotErr error
			output := captureCommandStderr(t, func() {
				gotErr = validateCommandArgs(cmd, test.path, test.input, globalOptions{})
			})
			if !errors.Is(gotErr, errBadArgs) {
				t.Fatalf("error = %v, want invalid arguments", gotErr)
			}
			if output != test.usage {
				t.Errorf("stderr = %q, want %q", output, test.usage)
			}
		})
	}
}

func TestNoteContentFlagRejectsImplicitStdin(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	restoreStdin := replaceNoteStdin(t, reader)
	defer restoreStdin()

	cmd, ok := lookupCommand("note update")
	if !ok {
		t.Fatal("note update not found")
	}
	var gotErr error
	output := captureCommandStderr(t, func() {
		gotErr = validateCommandArgs(cmd, "note update", []string{
			"notebook-1", "note-1", "--content", "body",
		}, globalOptions{})
	})
	if !errors.Is(gotErr, errBadArgs) {
		t.Fatalf("error = %v, want invalid arguments", gotErr)
	}
	const want = "usage: nlm note update <notebook-id> <note-id> [--title TITLE] [--content TEXT | --content-file FILE]\n"
	if output != want {
		t.Errorf("stderr = %q, want %q", output, want)
	}
}

func TestNoteMutationGraceWarningsAndMapping(t *testing.T) {
	devNull, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer devNull.Close()
	restoreStdin := replaceNoteStdin(t, devNull)
	defer restoreStdin()

	t.Run("create", func(t *testing.T) {
		parsed := parseNoteCommand(t, "note create", []string{"notebook-1", "Title", "Old body"})
		args, err := decodeNoteCreateArgs(parsed)
		if err != nil {
			t.Fatal(err)
		}
		var output bytes.Buffer
		var gotTitle, gotContent string
		if err := runNoteCreate(&output, strings.NewReader(""), args, func(title, content string) error {
			gotTitle, gotContent = title, content
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		const wantWarning = "nlm: 'note create <notebook-id> <title> <content>' is deprecated; use 'note create <notebook-id> <title> --content <content>'\n"
		if output.String() != wantWarning {
			t.Errorf("warning = %q, want %q", output.String(), wantWarning)
		}
		if gotTitle != "Title" || gotContent != "Old body" {
			t.Errorf("create = title %q, content %q", gotTitle, gotContent)
		}
	})

	t.Run("update", func(t *testing.T) {
		parsed := parseNoteCommand(t, "note update", []string{
			"notebook-1", "note-1", "Old body", "Old title",
		})
		args, err := decodeNoteUpdateArgs(parsed)
		if err != nil {
			t.Fatal(err)
		}
		var output bytes.Buffer
		var gotTitle, gotContent string
		if err := runNoteUpdate(
			&output,
			strings.NewReader(""),
			args,
			func(content, title string) error {
				gotTitle, gotContent = title, content
				return nil
			},
			func(_, _ *string) error { return errors.New("unexpected partial update") },
		); err != nil {
			t.Fatal(err)
		}
		const wantWarning = "nlm: 'note update <notebook-id> <note-id> <content> <title>' uses deprecated positional mutation fields; use 'note update <notebook-id> <note-id> --title <title> --content <content>'\n"
		if output.String() != wantWarning {
			t.Errorf("warning = %q, want %q", output.String(), wantWarning)
		}
		if gotTitle != "Old title" || gotContent != "Old body" {
			t.Errorf("update = title %q, content %q", gotTitle, gotContent)
		}
	})
}

func TestLegacyNoteMutationsKeepPositionalMapping(t *testing.T) {
	devNull, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer devNull.Close()
	restoreStdin := replaceNoteStdin(t, devNull)
	defer restoreStdin()

	createParsed := parseNoteCommand(t, "new-note", []string{"notebook-1", "Title", "Body"})
	createArgs, err := decodeNoteCreateArgs(createParsed)
	if err != nil {
		t.Fatal(err)
	}
	var createOutput bytes.Buffer
	var gotCreateTitle, gotCreateContent string
	if err := runNoteCreate(&createOutput, strings.NewReader(""), createArgs, func(title, content string) error {
		gotCreateTitle, gotCreateContent = title, content
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if createOutput.Len() != 0 || gotCreateTitle != "Title" || gotCreateContent != "Body" {
		t.Errorf("new-note output=%q title=%q content=%q", createOutput.String(), gotCreateTitle, gotCreateContent)
	}

	updateParsed := parseNoteCommand(t, "update-note", []string{
		"notebook-1", "note-1", "Body", "Title",
	})
	updateArgs, err := decodeNoteUpdateArgs(updateParsed)
	if err != nil {
		t.Fatal(err)
	}
	var updateOutput bytes.Buffer
	var gotUpdateTitle, gotUpdateContent string
	if err := runNoteUpdate(
		&updateOutput,
		strings.NewReader(""),
		updateArgs,
		func(content, title string) error {
			gotUpdateTitle, gotUpdateContent = title, content
			return nil
		},
		func(_, _ *string) error { return errors.New("unexpected partial update") },
	); err != nil {
		t.Fatal(err)
	}
	if updateOutput.Len() != 0 || gotUpdateTitle != "Title" || gotUpdateContent != "Body" {
		t.Errorf("update-note output=%q title=%q content=%q", updateOutput.String(), gotUpdateTitle, gotUpdateContent)
	}
}

func TestLegacyNoteMutationsRejectNamedFlags(t *testing.T) {
	devNull, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer devNull.Close()
	restoreStdin := replaceNoteStdin(t, devNull)
	defer restoreStdin()

	cmd, ok := lookupCommand("new-note")
	if !ok {
		t.Fatal("new-note not found")
	}
	if _, err := parseCommandSpec(cmd.spec, cmd.surfaceSpec, []string{
		"notebook-1", "Title", "--content", "Body",
	}, globalOptions{}); !errors.Is(err, errBadArgs) {
		t.Errorf("new-note error = %v, want invalid arguments", err)
	}

	cmd, ok = lookupCommand("update-note")
	if !ok {
		t.Fatal("update-note not found")
	}
	_, err = parseCommandSpec(cmd.spec, cmd.surfaceSpec, []string{
		"notebook-1", "note-1", "--title", "Title",
	}, globalOptions{})
	if !errors.Is(err, errBadArgs) {
		t.Errorf("update-note error = %v, want invalid arguments", err)
	}
}

func parseNoteCommand(t *testing.T, path string, input []string) parsedCommand {
	t.Helper()
	cmd, ok := lookupCommand(path)
	if !ok {
		t.Fatalf("%s not found", path)
	}
	parsed, err := parseCommandSpec(cmd.spec, cmd.surfaceSpec, input, globalOptions{})
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return parsed
}

func replaceNoteStdin(t *testing.T, stdin *os.File) func() {
	t.Helper()
	old := os.Stdin
	os.Stdin = stdin
	return func() {
		os.Stdin = old
	}
}

func noteString(value string) *string {
	return &value
}

func stringValue(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}
