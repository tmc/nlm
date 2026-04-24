package sync

import (
	"reflect"
	"testing"
)

func TestApplyExcludes(t *testing.T) {
	files := []string{
		"main.go",
		"pkg/foo.go",
		"pkg/foo.pb.go",
		"vendor/github.com/x/y.go",
		"docs/README.md",
	}
	tests := []struct {
		name     string
		patterns []string
		want     []string
	}{
		{
			name: "nil patterns is identity",
			want: files,
		},
		{
			name:     "basename glob skips generated files",
			patterns: []string{"*.pb.go"},
			want:     []string{"main.go", "pkg/foo.go", "vendor/github.com/x/y.go", "docs/README.md"},
		},
		{
			name:     "trailing slash matches a path prefix",
			patterns: []string{"vendor/"},
			want:     []string{"main.go", "pkg/foo.go", "pkg/foo.pb.go", "docs/README.md"},
		},
		{
			name:     "directory without slash matches prefix",
			patterns: []string{"docs"},
			want:     []string{"main.go", "pkg/foo.go", "pkg/foo.pb.go", "vendor/github.com/x/y.go"},
		},
		{
			name:     "multiple patterns compose",
			patterns: []string{"*.pb.go", "vendor/"},
			want:     []string{"main.go", "pkg/foo.go", "docs/README.md"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := append([]string(nil), files...)
			got, err := applyExcludes(in, tt.patterns)
			if err != nil {
				t.Fatalf("applyExcludes: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("applyExcludes(%v) = %v, want %v", tt.patterns, got, tt.want)
			}
		})
	}
}

func TestApplyExcludesBadPattern(t *testing.T) {
	_, err := applyExcludes([]string{"a.go"}, []string{"[unclosed"})
	if err == nil {
		t.Fatal("expected error for malformed pattern, got nil")
	}
}
