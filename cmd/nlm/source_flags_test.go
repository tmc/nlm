package main

import (
	"reflect"
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
			name:    "missing source",
			args:    []string{"nb"},
			wantErr: "missing notebook id or source",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sourceName, mimeType, replaceSourceID = "", "", ""
			gotOpts, gotNotebook, gotInputs, err := parseSourceAddArgs(tt.args)
			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr {
					t.Fatalf("parseSourceAddArgs(%q) error = %v, want %q", tt.args, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseSourceAddArgs(%q) error = %v", tt.args, err)
			}
			if gotOpts != tt.wantOpts {
				t.Fatalf("parseSourceAddArgs(%q) opts = %+v, want %+v", tt.args, gotOpts, tt.wantOpts)
			}
			if gotNotebook != tt.wantNotebook {
				t.Fatalf("parseSourceAddArgs(%q) notebook = %q, want %q", tt.args, gotNotebook, tt.wantNotebook)
			}
			if len(gotInputs) != len(tt.wantInputs) {
				t.Fatalf("parseSourceAddArgs(%q) inputs = %q, want %q", tt.args, gotInputs, tt.wantInputs)
			}
			for i := range gotInputs {
				if gotInputs[i] != tt.wantInputs[i] {
					t.Fatalf("parseSourceAddArgs(%q) inputs = %q, want %q", tt.args, gotInputs, tt.wantInputs)
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
			args:     []string{"nb", "./docs", "--force", "--json"},
			wantOpts: syncOptions{Force: true, JSON: true},
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
			sourceName, force, dryRun, maxBytes, jsonOutput = "", false, false, 0, false
			gotOpts, gotPos, err := parseSourceSyncArgs(tt.args)
			if tt.wantErrText != "" {
				if err == nil || err.Error() != tt.wantErrText {
					t.Fatalf("parseSourceSyncArgs(%q) error = %v, want %q", tt.args, err, tt.wantErrText)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseSourceSyncArgs(%q) error = %v", tt.args, err)
			}
			if !reflect.DeepEqual(gotOpts, tt.wantOpts) {
				t.Fatalf("parseSourceSyncArgs(%q) opts = %+v, want %+v", tt.args, gotOpts, tt.wantOpts)
			}
			if len(gotPos) != len(tt.wantPos) {
				t.Fatalf("parseSourceSyncArgs(%q) positional = %q, want %q", tt.args, gotPos, tt.wantPos)
			}
			for i := range gotPos {
				if gotPos[i] != tt.wantPos[i] {
					t.Fatalf("parseSourceSyncArgs(%q) positional = %q, want %q", tt.args, gotPos, tt.wantPos)
				}
			}
		})
	}
}

func TestParseSourcePackArgs(t *testing.T) {
	sourceName, maxBytes, packChunk = "", 0, 0
	gotOpts, gotPaths, err := parseSourcePackArgs([]string{"./docs", "--chunk", "2", "--name", "bundle"})
	if err != nil {
		t.Fatalf("parseSourcePackArgs() error = %v", err)
	}
	wantOpts := syncPackOptions{Name: "bundle", Chunk: 2}
	if !reflect.DeepEqual(gotOpts, wantOpts) {
		t.Fatalf("parseSourcePackArgs() opts = %+v, want %+v", gotOpts, wantOpts)
	}
	if len(gotPaths) != 1 || gotPaths[0] != "./docs" {
		t.Fatalf("parseSourcePackArgs() paths = %q, want [./docs]", gotPaths)
	}
}
