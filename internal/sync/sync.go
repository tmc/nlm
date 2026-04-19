package sync

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/tools/txtar"
)

// Source is a notebook source as returned by the server.
type Source struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// Client is the set of notebook operations that sync needs.
type Client interface {
	ListSources(ctx context.Context, notebookID string) ([]Source, error)
	AddSource(ctx context.Context, notebookID string, title string, r io.Reader) (string, error)
	DeleteSources(ctx context.Context, notebookID string, ids []string) error
	RenameSource(ctx context.Context, sourceID string, title string) error
}

// Options controls sync behavior.
type Options struct {
	MaxBytes int    // chunk threshold; 0 means 5120000
	Name     string // source name; required if ambiguous
	Force    bool   // re-upload even if hash unchanged
	DryRun   bool   // print plan, don't upload
	JSON     bool   // NDJSON output
}

func (o *Options) maxBytes() int {
	if o.MaxBytes <= 0 {
		return 5120000
	}
	return o.MaxBytes
}

// Pack discovers files, bundles them into txtar chunks, and returns the
// chunk bytes and their corresponding source names. It runs the same
// discover/quote/bundle pipeline as Run but performs no network I/O.
// Intended for preview (`nlm sync-pack`) and for tests.
func Pack(paths []string, opts Options) (chunks [][]byte, names []string, err error) {
	name, err := resolveName(opts.Name, paths)
	if err != nil {
		return nil, nil, err
	}
	files, err := discoverFiles(paths)
	if err != nil {
		return nil, nil, fmt.Errorf("discover files: %w", err)
	}
	if len(files) == 0 {
		return nil, nil, fmt.Errorf("no files found")
	}
	chunks, err = bundle(files, opts.maxBytes())
	if err != nil {
		return nil, nil, fmt.Errorf("bundle: %w", err)
	}
	return chunks, chunkNames(name, len(chunks)), nil
}

// Run syncs a single named source to a notebook.
//
// paths is a list of files and/or directories. Directories are expanded
// via git ls-files. All files are bundled into txtar, chunked at
// opts.MaxBytes, and uploaded as "name", "name (pt2)", etc.
//
// If paths is nil, file paths are read from stdin (one per line).
func Run(ctx context.Context, c Client, notebookID string, paths []string, opts Options, w io.Writer) error {
	name, err := resolveName(opts.Name, paths)
	if err != nil {
		return err
	}

	// Discover files.
	files, err := discoverFiles(paths)
	if err != nil {
		return fmt.Errorf("discover files: %w", err)
	}
	if len(files) == 0 {
		return fmt.Errorf("no files found")
	}

	// Bundle into txtar chunks.
	chunks, err := bundle(files, opts.maxBytes())
	if err != nil {
		return fmt.Errorf("bundle: %w", err)
	}

	// Name each chunk.
	names := chunkNames(name, len(chunks))

	// Hash each chunk.
	hashes := make([]string, len(chunks))
	for i, data := range chunks {
		h := sha256.Sum256(data)
		hashes[i] = fmt.Sprintf("%x", h)
	}

	// Load caches.
	hc := newHashCache(notebookID)
	sc := newSourceCache()

	// Fetch source list (with cache).
	sources, ok := sc.load(notebookID)
	if !ok {
		sources, err = c.ListSources(ctx, notebookID)
		if err != nil {
			return fmt.Errorf("list sources: %w", err)
		}
		_ = sc.save(notebookID, sources)
	}

	// Build title→source index.
	byTitle := make(map[string]Source)
	for _, s := range sources {
		byTitle[s.Title] = s
	}

	out := &outputWriter{w: w, json: opts.JSON}

	// Upload each chunk. Brief pause between API calls to avoid server 500s.
	uploaded := 0
	for i, data := range chunks {
		chunkName := names[i]
		hash := hashes[i]

		existing, exists := byTitle[chunkName]

		// Skip if unchanged.
		if !opts.Force && !hc.changed(chunkName, hash) {
			out.emit(event{Action: "skip", Name: chunkName, Reason: "unchanged"})
			continue
		}

		if opts.DryRun {
			action := "upload"
			if exists {
				action = "replace"
			}
			out.emit(event{Action: action, Name: chunkName, Bytes: len(data), DryRun: true})
			continue
		}

		if exists {
			// Gap-free replacement: rename old → upload new → delete old.
			oldName := chunkName + " [old]"
			if err := c.RenameSource(ctx, existing.ID, oldName); err != nil {
				return fmt.Errorf("rename %q: %w", chunkName, err)
			}

			newID, err := c.AddSource(ctx, notebookID, chunkName, strings.NewReader(string(data)))
			if err != nil {
				// Try to restore the old name on failure.
				_ = c.RenameSource(ctx, existing.ID, chunkName)
				return fmt.Errorf("upload %q: %w", chunkName, err)
			}

			if err := c.DeleteSources(ctx, notebookID, []string{existing.ID}); err != nil {
				fmt.Fprintf(os.Stderr, "warning: uploaded %s but failed to delete old %s: %v\n", newID, existing.ID, err)
			}

			_ = hc.save(chunkName, hash)
			sc.remove(notebookID, existing.ID)
			sc.append(notebookID, Source{ID: newID, Title: chunkName})
			out.emit(event{Action: "replace", Name: chunkName, SourceID: newID, OldID: existing.ID, Bytes: len(data)})
			uploaded++
		} else {
			// New source.
			newID, err := c.AddSource(ctx, notebookID, chunkName, strings.NewReader(string(data)))
			if err != nil {
				return fmt.Errorf("upload %q: %w", chunkName, err)
			}

			_ = hc.save(chunkName, hash)
			sc.append(notebookID, Source{ID: newID, Title: chunkName})
			out.emit(event{Action: "upload", Name: chunkName, SourceID: newID, Bytes: len(data)})
			uploaded++
		}
		// Brief pause between sequential uploads to avoid server rate limits.
		if uploaded > 0 && i < len(chunks)-1 {
			time.Sleep(time.Second)
		}
	}

	// Delete orphaned parts. Scan all sources for parts beyond what we
	// just uploaded. This handles chunk size changes gracefully — if a
	// source previously had 11 parts at 450KB and now has 1 at 5MB,
	// all 10 orphaned parts get cleaned up regardless of gaps.
	activeNames := make(map[string]bool, len(names))
	for _, n := range names {
		activeNames[n] = true
	}
	for title, src := range byTitle {
		if activeNames[title] {
			continue
		}
		if !isPartOf(title, name) {
			continue
		}
		if opts.DryRun {
			out.emit(event{Action: "delete", Name: title, OldID: src.ID, Reason: "orphan"})
			continue
		}
		if err := c.DeleteSources(ctx, notebookID, []string{src.ID}); err != nil {
			return fmt.Errorf("delete orphan %q: %w", title, err)
		}
		sc.remove(notebookID, src.ID)
		out.emit(event{Action: "delete", Name: title, OldID: src.ID, Reason: "orphan"})
	}

	return nil
}

// resolveName determines the source name.
func resolveName(name string, paths []string) (string, error) {
	if name != "" {
		return name, nil
	}
	if len(paths) == 1 {
		info, err := os.Stat(paths[0])
		if err == nil && info.IsDir() {
			return filepath.Base(paths[0]), nil
		}
		// Single file: use its basename without extension.
		return filepath.Base(paths[0]), nil
	}
	return "", fmt.Errorf("--name is required when multiple paths or stdin are used")
}

// discoverFiles expands paths into a flat file list.
// nil paths means read from stdin.
func discoverFiles(paths []string) ([]string, error) {
	if paths == nil {
		return readStdinPaths()
	}
	var files []string
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			return nil, fmt.Errorf("stat %s: %w", p, err)
		}
		if info.IsDir() {
			dirFiles, err := gitFiles(p)
			if err != nil {
				return nil, fmt.Errorf("list files in %s: %w", p, err)
			}
			files = append(files, dirFiles...)
		} else {
			files = append(files, p)
		}
	}
	return files, nil
}

func readStdinPaths() ([]string, error) {
	var paths []string
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			paths = append(paths, line)
		}
	}
	return paths, scanner.Err()
}

// bundle groups files into txtar chunks, each under maxBytes.
// Files larger than maxBytes get their own chunk (unavoidable).
//
// Files whose contents contain lines that look like txtar markers
// ("-- name --") are quoted with a '>'-prefix scheme matching
// txtar-c -quote, and the archive comment records "unquote NAME"
// directives so readers can recover the original bytes.
func bundle(files []string, maxBytes int) ([][]byte, error) {
	var chunks [][]byte
	var ar txtar.Archive
	currentSize := 0

	flush := func() {
		if len(ar.Files) > 0 {
			chunks = append(chunks, txtar.Format(&ar))
			ar = txtar.Archive{}
			currentSize = 0
		}
	}

	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", f, err)
		}

		// Skip binary files — txtar is text-only.
		if isBinary(data) {
			continue
		}

		// Quote files that contain embedded txtar markers so they
		// don't break the outer archive boundaries.
		quoted := false
		if needsQuote(data) {
			if len(data) > 0 && data[len(data)-1] != '\n' {
				data = append(data, '\n')
			}
			q, qerr := quote(data)
			if qerr != nil {
				return nil, fmt.Errorf("quote %s: %w", f, qerr)
			}
			data = q
			quoted = true
		}

		entrySize := len(f) + len(data) + 20 // approximate txtar overhead

		// Flush current chunk if adding this file would exceed the limit.
		if currentSize+entrySize > maxBytes {
			flush()
		}

		if quoted {
			ar.Comment = append(ar.Comment, []byte("unquote "+f+"\n")...)
		}
		ar.Files = append(ar.Files, txtar.File{
			Name: f,
			Data: data,
		})
		currentSize += entrySize

		// If this single file already exceeds maxBytes, flush it as
		// its own chunk so subsequent files start a fresh one.
		if currentSize >= maxBytes {
			flush()
		}
	}
	flush()
	return chunks, nil
}

// isBinary reports whether data looks like binary content.
// Uses net/http content sniffing on the first 512 bytes.
func isBinary(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	sniff := data
	if len(sniff) > 512 {
		sniff = sniff[:512]
	}
	ct := http.DetectContentType(sniff)
	return !strings.HasPrefix(ct, "text/") && ct != "application/json"
}

// isPartOf reports whether title is the base name or a chunk part of it.
// Matches "name" and "name (ptN)" for any N.
func isPartOf(title, name string) bool {
	if title == name {
		return true
	}
	if !strings.HasPrefix(title, name+" (pt") || !strings.HasSuffix(title, ")") {
		return false
	}
	mid := title[len(name)+4 : len(title)-1]
	for _, c := range mid {
		if c < '0' || c > '9' {
			return false
		}
	}
	return len(mid) > 0
}

// chunkNames returns the names for n chunks.
// First chunk: "name", second: "name (pt2)", etc.
func chunkNames(name string, n int) []string {
	names := make([]string, n)
	for i := range names {
		if i == 0 {
			names[i] = name
		} else {
			names[i] = fmt.Sprintf("%s (pt%d)", name, i+1)
		}
	}
	return names
}

// event is an NDJSON progress event.
type event struct {
	Action   string `json:"action"`
	Name     string `json:"name"`
	SourceID string `json:"source_id,omitempty"`
	OldID    string `json:"old_id,omitempty"`
	Bytes    int    `json:"bytes,omitempty"`
	Reason   string `json:"reason,omitempty"`
	DryRun   bool   `json:"dry_run,omitempty"`
}

type outputWriter struct {
	w    io.Writer
	json bool
}

func (o *outputWriter) emit(e event) {
	if o.json {
		data, _ := json.Marshal(e)
		fmt.Fprintln(o.w, string(data))
		return
	}
	switch e.Action {
	case "skip":
		fmt.Fprintf(os.Stderr, "  skip: %s (%s)\n", e.Name, e.Reason)
	case "upload":
		if e.DryRun {
			fmt.Fprintf(os.Stderr, "  would upload: %s (%d bytes)\n", e.Name, e.Bytes)
		} else {
			fmt.Fprintf(os.Stderr, "  upload: %s -> %s (%d bytes)\n", e.Name, e.SourceID, e.Bytes)
			fmt.Fprintln(o.w, e.SourceID)
		}
	case "replace":
		if e.DryRun {
			fmt.Fprintf(os.Stderr, "  would replace: %s (%d bytes)\n", e.Name, e.Bytes)
		} else {
			fmt.Fprintf(os.Stderr, "  replace: %s -> %s (was %s, %d bytes)\n", e.Name, e.SourceID, e.OldID, e.Bytes)
			fmt.Fprintln(o.w, e.SourceID)
		}
	case "delete":
		if e.DryRun {
			fmt.Fprintf(os.Stderr, "  would delete: %s (%s)\n", e.Name, e.Reason)
		} else {
			fmt.Fprintf(os.Stderr, "  delete: %s %s (%s)\n", e.Name, e.OldID, e.Reason)
		}
	}
}
