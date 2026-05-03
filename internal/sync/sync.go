package sync

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

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

// LabelPreserver is an optional capability: clients that implement it let
// sync carry label assignments across the replace path. The two-call shape
// matches the underlying RPCs (read all labels, attach one source per call).
type LabelPreserver interface {
	LabelsForSource(ctx context.Context, notebookID, sourceID string) ([]string, error)
	AttachLabelSource(ctx context.Context, notebookID, labelID, sourceID string) error
}

// Options controls sync behavior.
type Options struct {
	MaxBytes         int      // chunk threshold; 0 means 5120000
	Name             string   // source name; required if ambiguous
	Force            bool     // re-upload even if hash unchanged
	DryRun           bool     // print plan, don't upload
	JSON             bool     // NDJSON output
	Exclude          []string // filepath.Match patterns; files whose basename or full path matches are skipped
	IncludeUntracked bool     // when expanding git directories, include untracked non-ignored files
	Parallel         int      // max concurrent chunk uploads; 0 means 4, negative means serial
}

func (o *Options) maxBytes() int {
	if o.MaxBytes <= 0 {
		return 5120000
	}
	return o.MaxBytes
}

func (o *Options) parallel() int {
	if o.Parallel == 0 {
		return 4
	}
	if o.Parallel < 0 {
		return 1
	}
	return o.Parallel
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
	files, err := discoverFiles(paths, opts.IncludeUntracked)
	if err != nil {
		return nil, nil, fmt.Errorf("discover files: %w", err)
	}
	files, err = applyExcludes(files, opts.Exclude)
	if err != nil {
		return nil, nil, err
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
	files, err := discoverFiles(paths, opts.IncludeUntracked)
	if err != nil {
		return fmt.Errorf("discover files: %w", err)
	}
	files, err = applyExcludes(files, opts.Exclude)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("no files found")
	}

	// Bundle into txtar chunks.
	chunks, err := bundle(files, opts.maxBytes())
	if err != nil {
		return fmt.Errorf("bundle: %w", err)
	}
	if len(chunks) == 0 {
		return fmt.Errorf("no text files found")
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

	// Always fetch the live source list before a mutating sync. Hash-cache
	// skips are only correct if the remote source still exists.
	sources, err := c.ListSources(ctx, notebookID)
	if err != nil {
		return fmt.Errorf("list sources: %w", err)
	}
	_ = sc.save(notebookID, sources)

	// Build title→source index.
	byTitle := make(map[string]Source)
	for _, s := range sources {
		byTitle[s.Title] = s
	}

	out := &outputWriter{w: w, json: opts.JSON}

	// Plan: walk all chunks once and decide each chunk's action up front so
	// skip/dry-run output stays ordered. Real uploads run concurrently with
	// bounded parallelism. Each chunk targets a unique remote name, so
	// parallel uploads do not collide. State writes (hash cache, source
	// cache, output) go through mu.
	//
	// One chunk's failure must not abort the others: an in-flight rename or
	// upload aborted by a sibling's error tends to leave the notebook in a
	// half-renamed state ("name [old]" stranded). Independent chunks run to
	// completion and their errors are collected and returned together. The
	// caller's ctx (e.g. ^C) still cancels everything as expected — we only
	// avoid manufacturing cancellation from within.
	var (
		mu     sync.Mutex
		wg     sync.WaitGroup
		errsMu sync.Mutex
		errs   []error
	)
	sem := make(chan struct{}, opts.parallel())

	for i, data := range chunks {
		chunkName := names[i]
		hash := hashes[i]
		existing, exists := byTitle[chunkName]

		// Skip only when the hash is unchanged and the remote source is still
		// present under the expected title.
		if !opts.Force && exists && !hc.changed(chunkName, hash) {
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

		// Honor caller cancellation between dispatches without blocking on
		// the semaphore if ctx is already done. ctx.Done firing here means
		// the user (or a parent) cancelled us; record it once and stop
		// scheduling new work, but let already-running goroutines finish.
		select {
		case <-ctx.Done():
			errsMu.Lock()
			errs = append(errs, ctx.Err())
			errsMu.Unlock()
			goto wait
		case sem <- struct{}{}:
		}

		wg.Add(1)
		data, chunkName, hash, existing, exists := data, chunkName, hash, existing, exists
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			if err := uploadChunk(ctx, c, notebookID, chunkName, data, hash, existing, exists, hc, sc, out, &mu); err != nil {
				errsMu.Lock()
				errs = append(errs, err)
				errsMu.Unlock()
			}
		}()
	}
wait:
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		return err
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

// uploadChunk uploads or replaces a single chunk. It is safe to call from
// multiple goroutines because each chunk targets a unique remote name and
// shared state is updated under mu.
func uploadChunk(ctx context.Context, c Client, notebookID, chunkName string, data []byte, hash string, existing Source, exists bool, hc *hashCache, sc *sourceCache, out *outputWriter, mu *sync.Mutex) error {
	if !exists {
		newID, err := c.AddSource(ctx, notebookID, chunkName, strings.NewReader(string(data)))
		if err != nil {
			return fmt.Errorf("upload %q: %w", chunkName, err)
		}
		mu.Lock()
		_ = hc.save(chunkName, hash)
		sc.append(notebookID, Source{ID: newID, Title: chunkName})
		out.emit(event{Action: "upload", Name: chunkName, SourceID: newID, Bytes: len(data)})
		mu.Unlock()
		return nil
	}

	// Gap-free replacement: rename old → upload new → delete old.
	oldName := chunkName + " [old]"
	if err := c.RenameSource(ctx, existing.ID, oldName); err != nil {
		return fmt.Errorf("rename %q: %w", chunkName, err)
	}

	// Snapshot labels before delete; reattach after upload succeeds.
	var labelIDs []string
	if lp, ok := c.(LabelPreserver); ok {
		if got, err := lp.LabelsForSource(ctx, notebookID, existing.ID); err != nil {
			fmt.Fprintf(os.Stderr, "warning: read labels for %s: %v\n", existing.ID, err)
		} else {
			labelIDs = got
		}
	}

	newID, err := c.AddSource(ctx, notebookID, chunkName, strings.NewReader(string(data)))
	if err != nil {
		_ = c.RenameSource(ctx, existing.ID, chunkName)
		return fmt.Errorf("upload %q: %w", chunkName, err)
	}

	if err := c.DeleteSources(ctx, notebookID, []string{existing.ID}); err != nil {
		fmt.Fprintf(os.Stderr, "warning: uploaded %s but failed to delete old %s: %v\n", newID, existing.ID, err)
	}

	if len(labelIDs) > 0 {
		lp := c.(LabelPreserver)
		attached := 0
		for _, lid := range labelIDs {
			if err := lp.AttachLabelSource(ctx, notebookID, lid, newID); err != nil {
				fmt.Fprintf(os.Stderr, "warning: attach label %s to %s: %v\n", lid, newID, err)
				continue
			}
			attached++
		}
		if attached > 0 {
			fmt.Fprintf(os.Stderr, "  preserved %d label assignment(s) on %s\n", attached, chunkName)
		}
	}

	mu.Lock()
	_ = hc.save(chunkName, hash)
	sc.remove(notebookID, existing.ID)
	sc.append(notebookID, Source{ID: newID, Title: chunkName})
	out.emit(event{Action: "replace", Name: chunkName, SourceID: newID, OldID: existing.ID, Bytes: len(data)})
	mu.Unlock()
	return nil
}

// resolveName determines the source name.
func resolveName(name string, paths []string) (string, error) {
	if name != "" {
		return name, nil
	}
	if len(paths) == 1 {
		base := filepath.Base(paths[0])
		// "." / ".." / "" resolve to the CWD's basename, which is almost
		// always what the user meant.
		if base == "." || base == ".." || base == "" || base == string(filepath.Separator) {
			abs, err := filepath.Abs(paths[0])
			if err != nil {
				return "", fmt.Errorf("resolve default name from %q: %w", paths[0], err)
			}
			base = filepath.Base(abs)
		}
		return base, nil
	}
	return "", fmt.Errorf("--name is required when multiple paths or stdin are used")
}

// discoverFiles expands paths into a flat list of discovered files. Each
// entry carries both the on-disk path (for reading) and the member name
// (used as the txtar entry name on the wire). Directories are expanded via
// gitFiles, which keeps names repo-root-relative; explicit files use their
// basename so a single-file sync still produces a clean member name.
//
// nil paths means read from stdin.
func discoverFiles(paths []string, includeUntracked bool) ([]discovered, error) {
	if paths == nil {
		return readStdinPaths()
	}
	var files []discovered
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			return nil, fmt.Errorf("stat %s: %w", p, err)
		}
		if info.IsDir() {
			dirFiles, err := gitFiles(p, includeUntracked)
			if err != nil {
				return nil, fmt.Errorf("list files in %s: %w", p, err)
			}
			files = append(files, dirFiles...)
		} else {
			files = append(files, discovered{Path: p, Name: filepath.ToSlash(filepath.Base(p))})
		}
	}
	return files, nil
}

// readStdinPaths reads one path per line from stdin. Each line is treated
// as both the on-disk path and the member name, since the user has chosen
// the layout explicitly.
func readStdinPaths() ([]discovered, error) {
	var files []discovered
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			files = append(files, discovered{Path: line, Name: filepath.ToSlash(line)})
		}
	}
	return files, scanner.Err()
}

// bundle groups files into txtar chunks, each under maxBytes.
//
// Files whose contents contain lines that look like txtar markers
// ("-- name --") are quoted with a '>'-prefix scheme matching
// txtar-c -quote, and the archive comment records "unquote NAME"
// directives so readers can recover the original bytes.
//
// A single file whose serialized entry would exceed maxBytes is split
// across multiple chunks: each part becomes its own one-entry archive
// named "<original> (part i/N)" so the server-side per-source size limit
// is respected. Splits prefer line boundaries when possible.
func bundle(files []discovered, maxBytes int) ([][]byte, error) {
	// txtarOverhead is the approximate bytes added per entry: the marker
	// line "-- name --\n" plus a trailing newline. The slack covers the
	// archive-level newline and any small format quirks.
	const txtarOverhead = 20

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
		data, err := os.ReadFile(f.Path)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", f.Path, err)
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
				return nil, fmt.Errorf("quote %s: %w", f.Path, qerr)
			}
			data = q
			quoted = true
		}

		entrySize := len(f.Name) + len(data) + txtarOverhead

		// Oversize single file: flush whatever is pending, then emit one
		// chunk per part with a "(part i/N)" suffix on the entry name.
		if entrySize > maxBytes {
			flush()
			parts := splitFileData(data, maxBytes-len(f.Name)-len(" (part 99/99)")-txtarOverhead)
			for i, part := range parts {
				partName := fmt.Sprintf("%s (part %d/%d)", f.Name, i+1, len(parts))
				var partAr txtar.Archive
				if quoted {
					partAr.Comment = []byte("unquote " + partName + "\n")
				}
				partAr.Files = []txtar.File{{Name: partName, Data: part}}
				chunks = append(chunks, txtar.Format(&partAr))
			}
			continue
		}

		// Flush current chunk if adding this file would exceed the limit.
		if currentSize+entrySize > maxBytes {
			flush()
		}

		if quoted {
			ar.Comment = append(ar.Comment, []byte("unquote "+f.Name+"\n")...)
		}
		ar.Files = append(ar.Files, txtar.File{
			Name: f.Name,
			Data: data,
		})
		currentSize += entrySize
	}
	flush()
	return chunks, nil
}

// splitFileData splits data into pieces no larger than maxPart bytes each.
// It prefers to break on '\n' so a reader sees whole lines per part; if a
// run has no newline within maxPart bytes, it falls back to a hard byte
// split. maxPart must be at least 1.
func splitFileData(data []byte, maxPart int) [][]byte {
	if maxPart < 1 {
		maxPart = 1
	}
	var parts [][]byte
	for len(data) > maxPart {
		cut := maxPart
		// Walk back to the last newline within the window so the part
		// ends on a line boundary. Keep the newline with the earlier
		// part — the next part starts at the next byte.
		if nl := lastNewline(data[:cut]); nl >= 0 {
			cut = nl + 1
		}
		parts = append(parts, data[:cut])
		data = data[cut:]
	}
	if len(data) > 0 {
		parts = append(parts, data)
	}
	return parts
}

func lastNewline(data []byte) int {
	for i := len(data) - 1; i >= 0; i-- {
		if data[i] == '\n' {
			return i
		}
	}
	return -1
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
