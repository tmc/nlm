package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
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
// returned as-is — including a sole "-", which addSource interprets as
// "stream stdin as one source" (not as a line-delimited list).
func TestAddSourceInputs_PassThrough(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{"url and text literal", []string{"https://a/x", "topic"}, []string{"https://a/x", "topic"}},
		{"sole dash preserved for stdin blob", []string{"-"}, []string{"-"}},
		{"mixed with dash preserved (validation rejects later)", []string{"https://a/x", "-"}, []string{"https://a/x", "-"}},
		{"single file arg", []string{"./notes.md"}, []string{"./notes.md"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := addSourceInputs(tt.in)
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("addSourceInputs(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestStaleFailedAddSourceIDs(t *testing.T) {
	before := map[string]struct{}{
		"existing": {},
	}
	project := &pb.Project{
		Sources: []*pb.Source{
			{
				SourceId: &pb.SourceId{SourceId: "existing"},
				Settings: &pb.SourceSettings{Status: pb.SourceSettings_SOURCE_STATUS_ERROR},
			},
			{
				SourceId: &pb.SourceId{SourceId: "new-error"},
				Settings: &pb.SourceSettings{Status: pb.SourceSettings_SOURCE_STATUS_ERROR},
			},
			{
				SourceId: &pb.SourceId{SourceId: "new-ok"},
				Settings: &pb.SourceSettings{Status: pb.SourceSettings_SOURCE_STATUS_ENABLED},
			},
			{
				SourceId: &pb.SourceId{SourceId: "new-error-metadata"},
				Metadata: &pb.SourceMetadata{Status: pb.SourceSettings_SOURCE_STATUS_ERROR},
			},
		},
	}

	got := staleFailedAddSourceIDs(before, project)
	want := []string{"new-error", "new-error-metadata"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("staleFailedAddSourceIDs() = %v, want %v", got, want)
	}
}
