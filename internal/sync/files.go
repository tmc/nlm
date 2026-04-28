package sync

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// discovered is one file in the bundle: an absolute on-disk Path used to read
// the bytes, plus a Name that becomes the txtar member name on the wire.
// Names are kept relative to the user's bundle root (the git repo root when
// available, else the discovery directory) so citations resolve to short,
// portable paths instead of the syncing host's absolute layout.
type discovered struct {
	Path string
	Name string
}

// gitFiles returns tracked files under dir using git ls-files.
// Falls back to filepath.WalkDir if dir is not in a git repo.
//
// Member names are relative to the git repo root when available, so a
// directory deep inside a checkout still produces clean, portable txtar
// names like "cmd/nlm/main.go" rather than absolute paths.
//
// Index entries whose working-tree file is missing (deleted but not yet
// staged) are skipped with a stderr warning, so a single stale entry does
// not abort a multi-thousand-file sync.
func gitFiles(dir string) ([]discovered, error) {
	// --full-name returns paths relative to the repo root regardless of
	// where ls-files is invoked from, so a sync from cmd/nlm/ produces
	// "cmd/nlm/main.go" rather than "main.go". Pairing it with `git
	// rev-parse --show-toplevel` lets us reconstruct an absolute on-disk
	// path without depending on the caller's symlink resolution (macOS
	// /var vs /private/var) matching what git resolves internally.
	cmd := exec.Command("git", "ls-files", "--full-name", "-z")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return walkFiles(dir)
	}
	root := gitRoot(dir)
	if root == "" {
		root = dir
	}
	var files []discovered
	var missing []string
	for _, f := range strings.Split(string(out), "\000") {
		if f == "" {
			continue
		}
		path := filepath.Join(root, f)
		info, err := os.Lstat(path)
		if err != nil {
			if os.IsNotExist(err) {
				missing = append(missing, path)
				continue
			}
			return nil, fmt.Errorf("stat %s: %w", path, err)
		}
		if !info.Mode().IsRegular() {
			continue
		}
		files = append(files, discovered{Path: path, Name: filepath.ToSlash(f)})
	}
	if len(missing) > 0 {
		fmt.Fprintf(os.Stderr, "warning: skipping %d file(s) tracked by git but missing in working tree (deleted but not staged):\n", len(missing))
		for _, p := range missing {
			fmt.Fprintf(os.Stderr, "  %s\n", p)
		}
	}
	if len(files) == 0 {
		return walkFiles(dir)
	}
	return files, nil
}

// gitRoot returns the absolute path of the enclosing git repo's working
// tree, or "" if dir is not inside one.
func gitRoot(dir string) string {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// walkFiles returns all regular files under dir. Member names are relative
// to dir so the bundle contents look the same whether the user passed a
// short or long path.
func walkFiles(dir string) ([]discovered, error) {
	var files []discovered
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			base := d.Name()
			if base == ".git" || base == "node_modules" || base == "__pycache__" || base == ".eggs" {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		name, rerr := filepath.Rel(dir, path)
		if rerr != nil {
			name = path
		}
		files = append(files, discovered{Path: path, Name: filepath.ToSlash(name)})
		return nil
	})
	return files, err
}

// applyExcludes removes paths matching any of the given filepath.Match
// patterns. Each pattern is tried against both the full member name and
// its basename, so "*.pb.go" and "vendor/*" both work intuitively.
// Returns an error if a pattern is malformed.
func applyExcludes(files []discovered, patterns []string) ([]discovered, error) {
	if len(patterns) == 0 {
		return files, nil
	}
	for _, p := range patterns {
		if _, err := filepath.Match(p, ""); err != nil {
			return nil, fmt.Errorf("invalid --exclude pattern %q: %w", p, err)
		}
	}
	out := files[:0]
	for _, f := range files {
		if excluded(f.Name, patterns) {
			continue
		}
		out = append(out, f)
	}
	return out, nil
}

func excluded(path string, patterns []string) bool {
	base := filepath.Base(path)
	for _, p := range patterns {
		if ok, _ := filepath.Match(p, path); ok {
			return true
		}
		if ok, _ := filepath.Match(p, base); ok {
			return true
		}
		// Directory-style prefix match: "vendor/", "docs", or "pkg/internal".
		prefix := strings.TrimSuffix(p, "/")
		if prefix != "" && !strings.ContainsAny(prefix, "*?[") {
			if path == prefix || strings.HasPrefix(path, prefix+"/") {
				return true
			}
		}
	}
	return false
}
