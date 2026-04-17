package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestValidateSourceInputs covers the pre-RPC fail-fast rules. The bulk
// path is value only if callers can rely on an all-or-nothing contract; a
// partial batch would mean some IDs made it to the server while the tail
// did not, which is exactly the mode that breaks shell-pipe retry.
func TestValidateSourceInputs(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "hello.txt")
	if err := os.WriteFile(existing, []byte("hi"), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	missing := filepath.Join(dir, "does-not-exist.txt")

	tests := []struct {
		name    string
		inputs  []string
		wantErr string // substring; "" means expect nil
	}{
		{"empty batch", nil, "no sources provided"},
		{"one url", []string{"https://example.com/doc"}, ""},
		{"two urls", []string{"https://a/x", "http://b/y"}, ""},
		{"existing file", []string{existing}, ""},
		{"missing file with .txt suffix", []string{missing}, "does-not-exist.txt"},
		{"missing path with slash", []string{"/nope/path"}, "/nope/path"},
		{"text literal (no separator)", []string{"hello"}, ""},
		{"mixed urls and text", []string{"https://a/x", "topic"}, ""},
		{"bare dash in batch", []string{"https://a/x", "-"}, "stdin ('-')"},
		{"empty element", []string{"a", ""}, "empty source argument"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSourceInputs(tt.inputs)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("want nil, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("want error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error %q does not contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

// TestAddSourceInputs_PassThrough ensures literal argument lists are
// returned as-is (no accidental stdin consumption when the user passed
// explicit values).
func TestAddSourceInputs_PassThrough(t *testing.T) {
	got, err := addSourceInputs([]string{"https://a/x", "topic"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(got) != 2 || got[0] != "https://a/x" || got[1] != "topic" {
		t.Fatalf("pass-through mismatch: %#v", got)
	}
}
