package main

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestParseSourceAddArgs(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		wantOpts     sourceAddOptions
		wantNotebook string
		wantInputs   []string
		wantErr      string
	}{
		{
			name:         "flags before notebook",
			args:         []string{"--name", "API notes", "--mime", "text/plain", "nb", "-"},
			wantOpts:     sourceAddOptions{Name: "API notes", MIMEType: "text/plain"},
			wantNotebook: "nb",
			wantInputs:   []string{"-"},
		},
		{
			name:         "flags after positionals",
			args:         []string{"nb", "./notes.txt", "--replace", "src-1"},
			wantOpts:     sourceAddOptions{ReplaceSourceID: "src-1"},
			wantNotebook: "nb",
			wantInputs:   []string{"./notes.txt"},
		},
		{
			name:         "pre-process flag",
			args:         []string{"--pre-process", "tr a-z A-Z", "nb", "./notes.txt"},
			wantOpts:     sourceAddOptions{PreProcess: "tr a-z A-Z"},
			wantNotebook: "nb",
			wantInputs:   []string{"./notes.txt"},
		},
		{
			name:         "chunk flag",
			args:         []string{"--chunk", "5242880", "nb", "big.log"},
			wantOpts:     sourceAddOptions{Chunk: 5242880},
			wantNotebook: "nb",
			wantInputs:   []string{"big.log"},
		},
		{
			name:    "chunk above limit",
			args:    []string{"--chunk", "99999999", "nb", "x"},
			wantErr: "--chunk 99999999 exceeds per-request limit 10485760",
		},
		{
			name:    "chunk negative",
			args:    []string{"--chunk", "-1", "nb", "x"},
			wantErr: "--chunk must be >= 0",
		},
		{
			name:    "replace multiple sources",
			args:    []string{"nb", "a", "b", "--replace", "src-1"},
			wantErr: "--replace requires exactly one source",
		},
		{
			name:    "missing source",
			args:    []string{"nb"},
			wantErr: "missing notebook id or source",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := parseSourceCommand("source add", tt.args, globalOptions{})
			var got sourceAddArgs
			if err == nil {
				got, err = decodeSourceAddArgs(parsed)
			}
			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr {
					t.Fatalf("parse source add %q error = %v, want %q", tt.args, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parse source add %q error = %v", tt.args, err)
			}
			if got.Options != tt.wantOpts {
				t.Fatalf("parse source add %q options = %+v, want %+v", tt.args, got.Options, tt.wantOpts)
			}
			if got.NotebookID != tt.wantNotebook {
				t.Fatalf("parse source add %q notebook = %q, want %q", tt.args, got.NotebookID, tt.wantNotebook)
			}
			if len(got.Inputs) != len(tt.wantInputs) {
				t.Fatalf("parse source add %q inputs = %q, want %q", tt.args, got.Inputs, tt.wantInputs)
			}
			for i := range got.Inputs {
				if got.Inputs[i] != tt.wantInputs[i] {
					t.Fatalf("parse source add %q inputs = %q, want %q", tt.args, got.Inputs, tt.wantInputs)
				}
			}
		})
	}
}

func TestParseSourceSyncArgs(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		wantOpts    syncOptions
		wantPos     []string
		wantErrText string
	}{
		{
			name:     "flags after notebook",
			args:     []string{"nb", "./docs", "--force", "--json", "--include-untracked"},
			wantOpts: syncOptions{Force: true, JSON: true, IncludeUntracked: true},
			wantPos:  []string{"nb", "./docs"},
		},
		{
			name:        "missing notebook",
			args:        []string{"--force"},
			wantErrText: "missing notebook id",
		},
		{
			name:        "negative max bytes",
			args:        []string{"nb", "--max-bytes", "-1"},
			wantErrText: "--max-bytes must be >= 0",
		},
		{
			name:     "repeated exclude",
			args:     []string{"nb", "--exclude", "*.pb.go", "-x", "vendor/", "./src"},
			wantOpts: syncOptions{Exclude: []string{"*.pb.go", "vendor/"}},
			wantPos:  []string{"nb", "./src"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := parseSourceCommand("source sync", tt.args, globalOptions{})
			var got sourceSyncArgs
			if err == nil {
				got, err = decodeSourceSyncArgs(parsed)
			}
			if tt.wantErrText != "" {
				if err == nil || err.Error() != tt.wantErrText {
					t.Fatalf("parse source sync %q error = %v, want %q", tt.args, err, tt.wantErrText)
				}
				return
			}
			if err != nil {
				t.Fatalf("parse source sync %q error = %v", tt.args, err)
			}
			if !reflect.DeepEqual(got.Options, tt.wantOpts) {
				t.Fatalf("parse source sync %q options = %+v, want %+v", tt.args, got.Options, tt.wantOpts)
			}
			gotPos := append([]string{got.NotebookID}, got.Paths...)
			if len(gotPos) != len(tt.wantPos) {
				t.Fatalf("parse source sync %q positional = %q, want %q", tt.args, gotPos, tt.wantPos)
			}
			for i := range gotPos {
				if gotPos[i] != tt.wantPos[i] {
					t.Fatalf("parse source sync %q positional = %q, want %q", tt.args, gotPos, tt.wantPos)
				}
			}
		})
	}
}

func TestParseSourcePackArgs(t *testing.T) {
	parsed, err := parseSourceCommand("source pack", []string{"./docs", "--chunk", "2", "--name", "bundle"}, globalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeSourcePackArgs(parsed)
	if err != nil {
		t.Fatal(err)
	}
	wantOpts := syncPackOptions{Name: "bundle", Chunk: 2}
	if !reflect.DeepEqual(got.Options, wantOpts) {
		t.Fatalf("options = %+v, want %+v", got.Options, wantOpts)
	}
	if len(got.Paths) != 1 || got.Paths[0] != "./docs" {
		t.Fatalf("paths = %q, want [./docs]", got.Paths)
	}
}

func TestParseSourceArgsUseGlobalDefaults(t *testing.T) {
	globals := globalOptions{
		sourceName:      "docs",
		force:           true,
		jsonOutput:      true,
		maxBytes:        1024,
		replaceSourceID: "src-old",
	}
	parsed, err := parseSourceCommand("source sync", []string{"nb", "./docs"}, globals)
	if err != nil {
		t.Fatal(err)
	}
	syncArgs, err := decodeSourceSyncArgs(parsed)
	if err != nil {
		t.Fatal(err)
	}
	if !syncArgs.Options.Force || !syncArgs.Options.JSON || syncArgs.Options.Name != "docs" || syncArgs.Options.MaxBytes != 1024 {
		t.Fatalf("sync options = %+v, want inherited force/json/name/maxBytes", syncArgs.Options)
	}
	if got, want := strings.Join(append([]string{syncArgs.NotebookID}, syncArgs.Paths...), ","), "nb,./docs"; got != want {
		t.Fatalf("positional = %q, want %q", got, want)
	}

	parsed, err = parseSourceCommand("source add", []string{"nb", "README.md"}, globals)
	if err != nil {
		t.Fatal(err)
	}
	addArgs, err := decodeSourceAddArgs(parsed)
	if err != nil {
		t.Fatal(err)
	}
	if addArgs.Options.Name != "docs" || addArgs.Options.ReplaceSourceID != "src-old" {
		t.Fatalf("add options = %+v, want inherited name and replace id", addArgs.Options)
	}
}

func parseSourceCommand(path string, values []string, globals globalOptions) (parsedCommand, error) {
	command, ok := lookupCommand(path)
	if !ok {
		return parsedCommand{}, fmt.Errorf("command %q not found", path)
	}
	return parseCommandSpec(command.spec, command.surfaceSpec, values, globals)
}
