package sync

import (
	"os"
	"os/exec"
	"path/filepath"
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

// TestGitFilesSkipsDeletedUnstaged verifies that an index entry whose
// working-tree file has been removed is silently skipped, instead of
// aborting the whole file discovery. See the bug report at
// /tmp/collab-nlm-sync-bug.md for the originating repro.
func TestGitFilesSkipsDeletedUnstaged(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	run("init", "-q", "-b", "main")
	if err := os.WriteFile(filepath.Join(dir, "kept.go"), []byte("package a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "gone.go"), []byte("package a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "kept.go", "gone.go")
	run("commit", "-q", "-m", "init")

	// Delete in working tree only — git index still references it.
	if err := os.Remove(filepath.Join(dir, "gone.go")); err != nil {
		t.Fatal(err)
	}

	files, err := gitFiles(dir)
	if err != nil {
		t.Fatalf("gitFiles: %v", err)
	}
	want := []string{filepath.Join(dir, "kept.go")}
	if !reflect.DeepEqual(files, want) {
		t.Fatalf("gitFiles = %v, want %v", files, want)
	}
}
