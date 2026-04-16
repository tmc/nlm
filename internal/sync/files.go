package sync

import (
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
