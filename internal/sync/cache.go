package sync

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// hashCache stores content hashes for change detection.
// The zero value uses ~/.cache/nlm/sync/<notebookID>/ as the directory.
type hashCache struct {
	dir string
}

func newHashCache(notebookID string) *hashCache {
	home, _ := os.UserHomeDir()
	return &hashCache{
		dir: filepath.Join(home, ".cache", "nlm", "sync", notebookID),
	}
}

// changed reports whether the content hash differs from the cached value.
// Returns true if changed, no cache exists, or on any error.
func (c *hashCache) changed(name, hash string) bool {
	path := c.path(name)
	data, err := os.ReadFile(path)
	if err != nil {
		return true
	}
	return string(data) != hash
}

// save stores the hash for the given source name.
func (c *hashCache) save(name, hash string) error {
	if err := os.MkdirAll(c.dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(c.path(name), []byte(hash), 0o644)
}

func (c *hashCache) path(name string) string {
	h := sha256.Sum256([]byte(name))
	return filepath.Join(c.dir, fmt.Sprintf("%x", h))
}

// sourceCache caches the ListSources result for a notebook.
// Cache file: ~/.cache/nlm/sources/<notebook-id>.json
// TTL: 30 seconds.
type sourceCache struct {
	dir string
}

func newSourceCache() *sourceCache {
	home, _ := os.UserHomeDir()
	return &sourceCache{
		dir: filepath.Join(home, ".cache", "nlm", "sources"),
	}
}

func (c *sourceCache) path(notebookID string) string {
	return filepath.Join(c.dir, notebookID+".json")
}

func (c *sourceCache) load(notebookID string) ([]Source, bool) {
	path := c.path(notebookID)
	info, err := os.Stat(path)
	if err != nil {
		return nil, false
	}
	if time.Since(info.ModTime()) > 30*time.Second {
		return nil, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var sources []Source
	if err := json.Unmarshal(data, &sources); err != nil {
		return nil, false
	}
	return sources, true
}

func (c *sourceCache) save(notebookID string, sources []Source) error {
	if err := os.MkdirAll(c.dir, 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(sources)
	if err != nil {
		return err
	}
	return os.WriteFile(c.path(notebookID), data, 0o644)
}

// append adds a source to the cached list without re-fetching.
func (c *sourceCache) append(notebookID string, src Source) {
	sources, ok := c.load(notebookID)
	if !ok {
		sources = nil
	}
	sources = append(sources, src)
	_ = c.save(notebookID, sources)
}

// remove removes a source from the cached list by ID.
func (c *sourceCache) remove(notebookID string, id string) {
	sources, ok := c.load(notebookID)
	if !ok {
		return
	}
	filtered := sources[:0]
	for _, s := range sources {
		if s.ID != id {
			filtered = append(filtered, s)
		}
	}
	_ = c.save(notebookID, filtered)
}
