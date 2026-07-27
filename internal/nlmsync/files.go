package nlmsync

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
// If includeUntracked is true, untracked non-ignored files are included too.
// Falls back to filepath.WalkDir if dir is not in a git repo.
//
// Member names are relative to the git repo root when available, so a
// directory deep inside a checkout still produces clean, portable txtar
// names like "cmd/nlm/main.go" rather than absolute paths.
//
// Index entries whose working-tree file is missing (deleted but not yet
// staged) are skipped with a stderr warning, so a single stale entry does
// not abort a multi-thousand-file sync.
func gitFiles(dir string, includeUntracked bool) ([]discovered, error) {
	// --full-name returns paths relative to the repo root regardless of
	// where ls-files is invoked from, so a sync from cmd/nlm/ produces
	// "cmd/nlm/main.go" rather than "main.go". Pairing it with `git
	// rev-parse --show-toplevel` lets us reconstruct an absolute on-disk
	// path without depending on the caller's symlink resolution (macOS
	// /var vs /private/var) matching what git resolves internally.
	tracked, err := gitLsFiles(dir, "--full-name")
	if err != nil {
		return walkFiles(dir)
	}
	var names []string
	names = append(names, tracked...)
	if includeUntracked {
		untracked, err := gitLsFiles(dir, "--full-name", "--others", "--exclude-standard")
		if err != nil {
			return nil, fmt.Errorf("list untracked files in %s: %w", dir, err)
		}
		names = append(names, untracked...)
	}
	names = uniqueStrings(names)
	root := gitRoot(dir)
	if root == "" {
		root = dir
	}
	var files []discovered
	var missing []string
	for _, f := range names {
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

func gitLsFiles(dir string, args ...string) ([]string, error) {
	argv := []string{"ls-files"}
	argv = append(argv, args...)
	argv = append(argv, "-z")
	cmd := exec.Command("git", argv...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var files []string
	for _, f := range strings.Split(string(out), "\000") {
		if f == "" {
			continue
		}
		files = append(files, f)
	}
	return files, nil
}

func uniqueStrings(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
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

// ignoreFileName is the per-directory exclude file sync honors automatically,
// so a checkout can keep large or policy-sensitive files out of every sync
// without anyone having to remember the --exclude flag.
const ignoreFileName = ".nlmignore"

// mergeIgnores returns the explicit --exclude patterns combined with any
// patterns discovered in .nlmignore files under the synced paths. Explicit
// flags come first so their behavior is unchanged when no ignore file exists.
func mergeIgnores(paths, exclude []string) ([]string, error) {
	fromFile, err := loadNlmignore(paths)
	if err != nil {
		return nil, err
	}
	if len(fromFile) == 0 {
		return exclude, nil
	}
	merged := make([]string, 0, len(exclude)+len(fromFile))
	merged = append(merged, exclude...)
	merged = append(merged, fromFile...)
	return merged, nil
}

// loadNlmignore collects exclude patterns from a .nlmignore file for each
// directory in paths. The file is read from the git repo root (so its patterns
// match the repo-root-relative member names git ls-files produces), falling
// back to the directory itself outside a checkout. Each non-empty,
// non-comment line is one pattern, using the same matching as --exclude. A
// missing file is not an error; paths that are plain files are ignored here.
func loadNlmignore(paths []string) ([]string, error) {
	if paths == nil {
		return nil, nil
	}
	seenRoot := make(map[string]bool)
	var patterns []string
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil || !info.IsDir() {
			continue
		}
		root := gitRoot(p)
		if root == "" {
			root = p
		}
		if seenRoot[root] {
			continue
		}
		seenRoot[root] = true
		got, err := readIgnoreFile(filepath.Join(root, ignoreFileName))
		if err != nil {
			return nil, err
		}
		patterns = append(patterns, got...)
	}
	return patterns, nil
}

// readIgnoreFile parses an ignore file into exclude patterns. Blank lines and
// lines beginning with '#' are skipped; surrounding whitespace is trimmed. A
// non-existent file yields no patterns and no error.
func readIgnoreFile(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var patterns []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		patterns = append(patterns, line)
	}
	return patterns, nil
}
