package sync

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
)

func TestApplyExcludes(t *testing.T) {
	names := []string{
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
			want: names,
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
			in := make([]discovered, len(names))
			for i, n := range names {
				in[i] = discovered{Path: n, Name: n}
			}
			got, err := applyExcludes(in, tt.patterns)
			if err != nil {
				t.Fatalf("applyExcludes: %v", err)
			}
			gotNames := make([]string, len(got))
			for i, d := range got {
				gotNames[i] = d.Name
			}
			if !reflect.DeepEqual(gotNames, tt.want) {
				t.Fatalf("applyExcludes(%v) = %v, want %v", tt.patterns, gotNames, tt.want)
			}
		})
	}
}

func TestApplyExcludesBadPattern(t *testing.T) {
	_, err := applyExcludes([]discovered{{Path: "a.go", Name: "a.go"}}, []string{"[unclosed"})
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

	files, err := gitFiles(dir, false)
	if err != nil {
		t.Fatalf("gitFiles: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("got %d files, want 1: %+v", len(files), files)
	}
	got := files[0]
	if got.Name != "kept.go" {
		t.Errorf("Name = %q, want %q", got.Name, "kept.go")
	}
	// Path may be expressed via /private/var/... on macOS where t.TempDir
	// returns /var/folders/...; sameFile resolves both through stat.
	wantPath := filepath.Join(dir, "kept.go")
	if !sameFile(t, got.Path, wantPath) {
		t.Errorf("Path = %q, want path equivalent to %q", got.Path, wantPath)
	}
}

// sameFile reports whether a and b name the same file on disk, accounting
// for OS symlinks (macOS /var vs /private/var) so tests can write paths in
// one form and assert against returns from tools that resolve them.
func sameFile(t *testing.T, a, b string) bool {
	t.Helper()
	ai, err := os.Stat(a)
	if err != nil {
		return false
	}
	bi, err := os.Stat(b)
	if err != nil {
		return false
	}
	return os.SameFile(ai, bi)
}

// TestGitFilesNamesAreRepoRootRelative verifies that running gitFiles
// from a subdirectory of a repo still names files relative to the repo
// root. This is what makes the txtar member names portable: a sync run
// from cmd/nlm/ produces "cmd/nlm/main.go", not the absolute checkout
// path of the syncing host. The companion --resolve-citations feature
// relies on these short names to render readable file:line locations.
func TestGitFilesNamesAreRepoRootRelative(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()

	run := func(workdir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = workdir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	run(dir, "init", "-q", "-b", "main")
	sub := filepath.Join(dir, "cmd", "nlm")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(dir, "add", "-A")
	run(dir, "commit", "-q", "-m", "init")

	files, err := gitFiles(sub, false)
	if err != nil {
		t.Fatalf("gitFiles: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("got %d files, want 1: %+v", len(files), files)
	}
	got := files[0]
	wantPath := filepath.Join(sub, "main.go")
	if !sameFile(t, got.Path, wantPath) {
		t.Errorf("Path = %q, want path equivalent to %q", got.Path, wantPath)
	}
	// Name must be repo-root-relative ("cmd/nlm/main.go"), not
	// dir-relative ("main.go") and not absolute.
	if got.Name != "cmd/nlm/main.go" {
		t.Errorf("Name = %q, want %q", got.Name, "cmd/nlm/main.go")
	}
}

func TestDiscoverFilesIncludeUntracked(t *testing.T) {
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
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("ignored.txt\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tracked.txt"), []byte("tracked\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "untracked.txt"), []byte("untracked\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ignored.txt"), []byte("ignored\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".gitignore", "tracked.txt")
	run("commit", "-q", "-m", "init")

	t.Run("tracked file included by default", func(t *testing.T) {
		files, err := discoverFiles([]string{dir}, false)
		if err != nil {
			t.Fatalf("discoverFiles: %v", err)
		}
		if !containsName(files, "tracked.txt") {
			t.Fatalf("tracked.txt not found in %+v", files)
		}
	})

	t.Run("untracked file excluded by default", func(t *testing.T) {
		files, err := discoverFiles([]string{dir}, false)
		if err != nil {
			t.Fatalf("discoverFiles: %v", err)
		}
		if containsName(files, "untracked.txt") {
			t.Fatalf("untracked.txt unexpectedly found in %+v", files)
		}
	})

	t.Run("untracked file included with flag", func(t *testing.T) {
		files, err := discoverFiles([]string{dir}, true)
		if err != nil {
			t.Fatalf("discoverFiles: %v", err)
		}
		if !containsName(files, "untracked.txt") {
			t.Fatalf("untracked.txt not found in %+v", files)
		}
	})

	t.Run("ignored file excluded with flag", func(t *testing.T) {
		files, err := discoverFiles([]string{dir}, true)
		if err != nil {
			t.Fatalf("discoverFiles: %v", err)
		}
		if containsName(files, "ignored.txt") {
			t.Fatalf("ignored.txt unexpectedly found in %+v", files)
		}
	})

	t.Run("explicit untracked path works without flag", func(t *testing.T) {
		p := filepath.Join(dir, "untracked-explicit.txt")
		if err := os.WriteFile(p, []byte("explicit\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		files, err := discoverFiles([]string{p}, false)
		if err != nil {
			t.Fatalf("discoverFiles: %v", err)
		}
		if len(files) != 1 {
			t.Fatalf("got %d files, want 1: %+v", len(files), files)
		}
		if files[0].Name != "untracked-explicit.txt" {
			t.Fatalf("Name = %q, want %q", files[0].Name, "untracked-explicit.txt")
		}
	})
}

func containsName(files []discovered, name string) bool {
	for _, f := range files {
		if f.Name == name {
			return true
		}
	}
	return false
}
