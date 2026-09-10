package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// useTempCacheDir points os.UserCacheDir at a scratch directory for the test.
func useTempCacheDir(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(dir, "cache"))
}

func TestSourceTitleCacheRoundTrip(t *testing.T) {
	useTempCacheDir(t)
	want := map[string]string{"src-1": "Design notes", "src-2": ""}
	saveSourceTitles("nb-1", want, map[string]bool{"src-gone": true})
	got, absent := loadSourceTitles("nb-1")
	if !absent["src-gone"] || len(absent) != 1 {
		t.Fatalf("absent = %v, want {src-gone}", absent)
	}
	if len(got) != len(want) {
		t.Fatalf("loadSourceTitles = %v, want %v", got, want)
	}
	for id, title := range want {
		if got[id] != title {
			t.Fatalf("title[%q] = %q, want %q", id, got[id], title)
		}
	}
	if got, _ := loadSourceTitles("nb-2"); got != nil {
		t.Fatal("loadSourceTitles on another notebook = non-nil, want miss")
	}
}

func TestSourceTitleCacheExpires(t *testing.T) {
	useTempCacheDir(t)
	saveSourceTitles("nb-1", map[string]string{"src-1": "Design notes"}, nil)
	path := sourceTitleCacheFile("nb-1")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var entry sourceTitleCacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		t.Fatal(err)
	}
	entry.Fetched = time.Now().Add(-sourceTitleCacheTTL - time.Minute)
	data, err = json.Marshal(entry)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if got, _ := loadSourceTitles("nb-1"); got != nil {
		t.Fatalf("loadSourceTitles on expired entry = %v, want nil", got)
	}
}

func TestSourceTitleCacheRejectsUnsafeID(t *testing.T) {
	useTempCacheDir(t)
	for _, id := range []string{"", ".", "..", "a/b", "../escape"} {
		if got := sourceTitleCacheFile(id); got != "" {
			t.Fatalf("sourceTitleCacheFile(%q) = %q, want \"\"", id, got)
		}
	}
}

func TestSourceTitleCacheEmptyIsMiss(t *testing.T) {
	useTempCacheDir(t)
	saveSourceTitles("nb-1", nil, nil)
	if got, _ := loadSourceTitles("nb-1"); got != nil {
		t.Fatalf("loadSourceTitles after empty save = %v, want nil", got)
	}
}
