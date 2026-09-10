package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// sourceTitleCacheTTL bounds how long a cached notebook source list is served.
// Fetching that list costs a full GetProject round trip, which runs several
// seconds on a large notebook, so every render that resolves a citation title
// paid for it. Titles change rarely; an hour keeps repeat renders instant
// without letting a rename linger past a working session.
const sourceTitleCacheTTL = time.Hour

// sourceTitleCacheEntry is the on-disk form of one notebook's source titles.
// Absent holds source IDs a live fetch confirmed the notebook does not hold,
// so a later render can report them removed without refetching. An ID in
// neither map is unknown and forces a refetch.
type sourceTitleCacheEntry struct {
	Fetched time.Time         `json:"fetched"`
	Titles  map[string]string `json:"titles"`
	Absent  []string          `json:"absent,omitempty"`
}

// sourceTitleCacheDir returns the on-disk cache directory for notebook source
// titles, creating it on first use.
func sourceTitleCacheDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "nlm", "source-titles")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// sourceTitleCacheFile returns the cache path for projectID, or "" when the ID
// is not a plain path element and so cannot safely name a file.
func sourceTitleCacheFile(projectID string) string {
	if projectID == "" || projectID == "." || projectID == ".." ||
		strings.ContainsRune(projectID, filepath.Separator) || strings.ContainsRune(projectID, '/') {
		return ""
	}
	dir, err := sourceTitleCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, projectID+".json")
}

// loadSourceTitles returns the cached titles and confirmed-absent IDs for
// projectID, or (nil, nil) on a miss, an expired entry, or any read error. A
// cache failure must never break a render, so no error is reported: the caller
// refetches.
func loadSourceTitles(projectID string) (titles map[string]string, absent map[string]bool) {
	path := sourceTitleCacheFile(projectID)
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil
	}
	var entry sourceTitleCacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, nil
	}
	if len(entry.Titles) == 0 || time.Since(entry.Fetched) > sourceTitleCacheTTL {
		return nil, nil
	}
	absent = make(map[string]bool, len(entry.Absent))
	for _, id := range entry.Absent {
		absent[id] = true
	}
	return entry.Titles, absent
}

// saveSourceTitles records titles and confirmed-absent IDs for projectID.
// Errors are dropped: a cache that cannot be written is a slow render, not a
// failed one.
func saveSourceTitles(projectID string, titles map[string]string, absent map[string]bool) {
	path := sourceTitleCacheFile(projectID)
	if path == "" || len(titles) == 0 {
		return
	}
	entry := sourceTitleCacheEntry{Fetched: time.Now(), Titles: titles}
	for id := range absent {
		entry.Absent = append(entry.Absent, id)
	}
	sort.Strings(entry.Absent)
	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
	}
}
