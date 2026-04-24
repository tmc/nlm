package sync

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// gitFiles returns tracked files under dir using git ls-files.
// Falls back to filepath.WalkDir if dir is not in a git repo.
func gitFiles(dir string) ([]string, error) {
	cmd := exec.Command("git", "ls-files", "-z")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return walkFiles(dir)
	}
	var files []string
	for _, f := range strings.Split(string(out), "\000") {
		if f == "" {
			continue
		}
		files = append(files, filepath.Join(dir, f))
	}
	if len(files) == 0 {
		return walkFiles(dir)
	}
	return files, nil
}

// walkFiles returns all regular files under dir.
func walkFiles(dir string) ([]string, error) {
	var files []string
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
		if d.Type().IsRegular() {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

// applyExcludes removes paths matching any of the given filepath.Match
// patterns. Each pattern is tried against both the full path and the
// basename, so "*.pb.go" and "vendor/*" both work intuitively. Returns
// an error if a pattern is malformed.
func applyExcludes(files, patterns []string) ([]string, error) {
	if len(patterns) == 0 {
		return files, nil
	}
	// Validate patterns up front so a typo fails fast instead of silently
	// matching nothing.
	for _, p := range patterns {
		if _, err := filepath.Match(p, ""); err != nil {
			return nil, fmt.Errorf("invalid --exclude pattern %q: %w", p, err)
		}
	}
	out := files[:0]
	for _, f := range files {
		if excluded(f, patterns) {
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
			if path == prefix || strings.HasPrefix(path, prefix+string(filepath.Separator)) {
				return true
			}
		}
	}
	return false
}
