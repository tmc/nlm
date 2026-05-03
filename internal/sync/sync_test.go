package sync

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"golang.org/x/tools/txtar"
)

func setupTestHome(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
}

type fakeClient struct {
	mu       sync.Mutex
	sources  []Source
	listed   int
	uploaded []struct {
		title   string
		content string
	}
	renamed []struct {
		id    string
		title string
	}
	deleted []string
}

func (f *fakeClient) ListSources(_ context.Context, _ string) ([]Source, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.listed++
	return append([]Source(nil), f.sources...), nil
}

func (f *fakeClient) AddSource(_ context.Context, _ string, title string, r io.Reader) (string, error) {
	data, _ := io.ReadAll(r)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.uploaded = append(f.uploaded, struct {
		title   string
		content string
	}{title, string(data)})
	id := "src-" + title
	f.sources = append(f.sources, Source{ID: id, Title: title})
	return id, nil
}

func (f *fakeClient) DeleteSources(_ context.Context, _ string, ids []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleted = append(f.deleted, ids...)
	return nil
}

func (f *fakeClient) RenameSource(_ context.Context, id string, title string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.renamed = append(f.renamed, struct {
		id    string
		title string
	}{id, title})
	return nil
}

func TestRunNewSource(t *testing.T) {
	setupTestHome(t)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hello world"), 0o644)
	os.WriteFile(filepath.Join(dir, "foo.go"), []byte("package foo"), 0o644)

	fc := &fakeClient{}
	var buf bytes.Buffer
	err := Run(context.Background(), fc, "nb-123", []string{dir}, Options{Name: "test"}, &buf)
	if err != nil {
		t.Fatal(err)
	}

	if len(fc.uploaded) != 1 {
		t.Fatalf("expected 1 upload, got %d", len(fc.uploaded))
	}
	if fc.uploaded[0].title != "test" {
		t.Errorf("expected title 'test', got %q", fc.uploaded[0].title)
	}
	if !strings.Contains(fc.uploaded[0].content, "hello.txt") {
		t.Errorf("expected txtar to contain hello.txt")
	}
	if !strings.Contains(buf.String(), "src-test") {
		t.Errorf("expected source ID in output, got %q", buf.String())
	}
}

func TestRunReplaceSource(t *testing.T) {
	setupTestHome(t)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("updated"), 0o644)

	fc := &fakeClient{
		sources: []Source{{ID: "old-123", Title: "test"}},
	}
	var buf bytes.Buffer
	err := Run(context.Background(), fc, "nb-123", []string{dir}, Options{Name: "test", Force: true}, &buf)
	if err != nil {
		t.Fatal(err)
	}

	// Should have renamed old, uploaded new, deleted old.
	if len(fc.renamed) != 1 {
		t.Fatalf("expected 1 rename, got %d", len(fc.renamed))
	}
	if fc.renamed[0].title != "test [old]" {
		t.Errorf("expected rename to 'test [old]', got %q", fc.renamed[0].title)
	}
	if len(fc.uploaded) != 1 {
		t.Fatalf("expected 1 upload, got %d", len(fc.uploaded))
	}
	if len(fc.deleted) != 1 || fc.deleted[0] != "old-123" {
		t.Errorf("expected delete of old-123, got %v", fc.deleted)
	}
}

// fakeLabelClient extends fakeClient with the LabelPreserver capability. Each
// source carries its own label assignments; replace-on-existing must read
// before delete and reattach to the new ID.
type fakeLabelClient struct {
	*fakeClient
	labelsBySource map[string][]string // sourceID -> []labelID
	attachCalls    []struct {
		labelID  string
		sourceID string
	}
}

func (f *fakeLabelClient) LabelsForSource(_ context.Context, _, sourceID string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.labelsBySource[sourceID]...), nil
}

func (f *fakeLabelClient) AttachLabelSource(_ context.Context, _, labelID, sourceID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.attachCalls = append(f.attachCalls, struct {
		labelID  string
		sourceID string
	}{labelID, sourceID})
	f.labelsBySource[sourceID] = append(f.labelsBySource[sourceID], labelID)
	return nil
}

func TestRunReplacePreservesLabels(t *testing.T) {
	setupTestHome(t)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("updated"), 0o644)

	fc := &fakeLabelClient{
		fakeClient: &fakeClient{
			sources: []Source{{ID: "old-123", Title: "test"}},
		},
		labelsBySource: map[string][]string{
			"old-123": {"label-A", "label-B"},
		},
	}
	var buf bytes.Buffer
	err := Run(context.Background(), fc, "nb-123", []string{dir}, Options{Name: "test", Force: true}, &buf)
	if err != nil {
		t.Fatal(err)
	}

	if len(fc.uploaded) != 1 {
		t.Fatalf("expected 1 upload, got %d", len(fc.uploaded))
	}
	newID := "src-test"
	if len(fc.attachCalls) != 2 {
		t.Fatalf("expected 2 label attachments, got %d", len(fc.attachCalls))
	}
	for _, call := range fc.attachCalls {
		if call.sourceID != newID {
			t.Errorf("attach went to %q, want %q", call.sourceID, newID)
		}
	}
	got := append([]string(nil), fc.attachCalls[0].labelID, fc.attachCalls[1].labelID)
	if got[0] != "label-A" || got[1] != "label-B" {
		t.Errorf("attached labels = %v, want [label-A label-B]", got)
	}
}

func TestRunReplaceWithoutLabelPreserverIsBackcompat(t *testing.T) {
	setupTestHome(t)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("updated"), 0o644)

	fc := &fakeClient{sources: []Source{{ID: "old-123", Title: "test"}}}
	var buf bytes.Buffer
	if err := Run(context.Background(), fc, "nb-123", []string{dir}, Options{Name: "test", Force: true}, &buf); err != nil {
		t.Fatal(err)
	}
	if len(fc.uploaded) != 1 || len(fc.deleted) != 1 {
		t.Fatalf("plain replace path broken: uploads=%d deletes=%d", len(fc.uploaded), len(fc.deleted))
	}
}

func TestRunSkipUnchanged(t *testing.T) {
	setupTestHome(t)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("same"), 0o644)

	fc := &fakeClient{}
	var buf bytes.Buffer

	// First run: upload.
	err := Run(context.Background(), fc, "nb-skip", []string{dir}, Options{Name: "test"}, &buf)
	if err != nil {
		t.Fatal(err)
	}
	if len(fc.uploaded) != 1 {
		t.Fatalf("expected 1 upload on first run, got %d", len(fc.uploaded))
	}

	// Second run: should skip.
	fc2 := &fakeClient{sources: fc.sources}
	buf.Reset()
	err = Run(context.Background(), fc2, "nb-skip", []string{dir}, Options{Name: "test"}, &buf)
	if err != nil {
		t.Fatal(err)
	}
	if len(fc2.uploaded) != 0 {
		t.Errorf("expected 0 uploads on second run, got %d", len(fc2.uploaded))
	}
}

func TestRunReuploadsWhenSourceMissingRemotely(t *testing.T) {
	setupTestHome(t)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("same"), 0o644)

	fc := &fakeClient{}
	var buf bytes.Buffer

	// First run seeds the hash cache and the source cache.
	if err := Run(context.Background(), fc, "nb-missing", []string{dir}, Options{Name: "test"}, &buf); err != nil {
		t.Fatal(err)
	}
	if len(fc.uploaded) != 1 {
		t.Fatalf("expected 1 upload on first run, got %d", len(fc.uploaded))
	}

	// Second run sees the same content hash, but the source is gone on the
	// server. Sync must upload it again instead of trusting local caches.
	fc2 := &fakeClient{}
	buf.Reset()
	if err := Run(context.Background(), fc2, "nb-missing", []string{dir}, Options{Name: "test"}, &buf); err != nil {
		t.Fatal(err)
	}
	if fc2.listed == 0 {
		t.Fatal("expected live source list fetch on second run")
	}
	if len(fc2.uploaded) != 1 {
		t.Fatalf("expected re-upload when remote source is missing, got %d uploads", len(fc2.uploaded))
	}
}

func TestRunDryRun(t *testing.T) {
	setupTestHome(t)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("content"), 0o644)

	fc := &fakeClient{}
	var buf bytes.Buffer
	err := Run(context.Background(), fc, "nb-123", []string{dir}, Options{Name: "test", DryRun: true}, &buf)
	if err != nil {
		t.Fatal(err)
	}
	if len(fc.uploaded) != 0 {
		t.Errorf("dry run should not upload, got %d uploads", len(fc.uploaded))
	}
}

func TestRunBinaryOnlyDoesNotDeleteExistingSources(t *testing.T) {
	setupTestHome(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "image.bin"), []byte{0, 1, 2, 3}, 0o644); err != nil {
		t.Fatal(err)
	}

	fc := &fakeClient{
		sources: []Source{
			{ID: "old-1", Title: "test"},
			{ID: "old-2", Title: "test (pt2)"},
		},
	}
	var buf bytes.Buffer
	err := Run(context.Background(), fc, "nb-123", []string{dir}, Options{Name: "test"}, &buf)
	if err == nil {
		t.Fatal("expected error for binary-only sync")
	}
	if !strings.Contains(err.Error(), "no text files found") {
		t.Fatalf("error = %v, want no text files found", err)
	}
	if len(fc.uploaded) != 0 {
		t.Fatalf("expected no uploads, got %d", len(fc.uploaded))
	}
	if len(fc.deleted) != 0 {
		t.Fatalf("expected no deletes, got %v", fc.deleted)
	}
}

func TestRunJSON(t *testing.T) {
	setupTestHome(t)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("content"), 0o644)

	fc := &fakeClient{}
	var buf bytes.Buffer
	err := Run(context.Background(), fc, "nb-123", []string{dir}, Options{Name: "test", JSON: true}, &buf)
	if err != nil {
		t.Fatal(err)
	}

	var ev event
	if err := json.Unmarshal([]byte(strings.TrimSpace(buf.String())), &ev); err != nil {
		t.Fatalf("expected NDJSON output, got %q: %v", buf.String(), err)
	}
	if ev.Action != "upload" {
		t.Errorf("expected action 'upload', got %q", ev.Action)
	}
}

func TestRunOrphanCleanup(t *testing.T) {
	setupTestHome(t)
	dir := t.TempDir()
	// Small file — will be a single chunk.
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("small"), 0o644)

	// Simulate existing sources that were 5 parts at old chunk size.
	fc := &fakeClient{
		sources: []Source{
			{ID: "id-0", Title: "test"},
			{ID: "id-2", Title: "test (pt2)"},
			{ID: "id-3", Title: "test (pt3)"},
			{ID: "id-4", Title: "test (pt4)"},
			{ID: "id-5", Title: "test (pt5)"},
			{ID: "other", Title: "unrelated source"},
		},
	}
	var buf bytes.Buffer
	err := Run(context.Background(), fc, "nb-orphan", []string{dir}, Options{Name: "test", Force: true}, &buf)
	if err != nil {
		t.Fatal(err)
	}

	// Should have replaced "test" and deleted pt2-pt5 as orphans.
	// "unrelated source" must NOT be deleted.
	if len(fc.deleted) != 5 { // old "test" + pt2 + pt3 + pt4 + pt5
		t.Errorf("expected 5 deletes (1 replaced + 4 orphans), got %d: %v", len(fc.deleted), fc.deleted)
	}
	for _, id := range fc.deleted {
		if id == "other" {
			t.Error("deleted unrelated source")
		}
	}
}

func TestIsPartOf(t *testing.T) {
	tests := []struct {
		title string
		name  string
		want  bool
	}{
		{"foo", "foo", true},
		{"foo (pt2)", "foo", true},
		{"foo (pt10)", "foo", true},
		{"foo (pt)", "foo", false},
		{"foo (ptx)", "foo", false},
		{"bar", "foo", false},
		{"foo: extra", "foo", false},
		{"foo (pt2) extra", "foo", false},
	}
	for _, tt := range tests {
		got := isPartOf(tt.title, tt.name)
		if got != tt.want {
			t.Errorf("isPartOf(%q, %q) = %v, want %v", tt.title, tt.name, got, tt.want)
		}
	}
}

func TestChunkNames(t *testing.T) {
	tests := []struct {
		name string
		n    int
		want []string
	}{
		{"foo", 1, []string{"foo"}},
		{"bar", 3, []string{"bar", "bar (pt2)", "bar (pt3)"}},
	}
	for _, tt := range tests {
		got := chunkNames(tt.name, tt.n)
		if len(got) != len(tt.want) {
			t.Errorf("chunkNames(%q, %d) = %v, want %v", tt.name, tt.n, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("chunkNames(%q, %d)[%d] = %q, want %q", tt.name, tt.n, i, got[i], tt.want[i])
			}
		}
	}
}

// flakyClient fails uploads whose title matches a substring; all other
// operations behave like fakeClient.
type flakyClient struct {
	*fakeClient
	failOnTitle string // substring match
}

func (f *flakyClient) AddSource(ctx context.Context, notebookID, title string, r io.Reader) (string, error) {
	if f.failOnTitle != "" && strings.Contains(title, f.failOnTitle) {
		_, _ = io.ReadAll(r) // drain
		return "", fmt.Errorf("simulated failure for %s", title)
	}
	return f.fakeClient.AddSource(ctx, notebookID, title, r)
}

func TestRunFailedChunkDoesNotCancelSiblings(t *testing.T) {
	setupTestHome(t)
	dir := t.TempDir()
	for i := range 6 {
		body := strings.Repeat(fmt.Sprintf("line %d\n", i), 200)
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("f%d.txt", i)), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Fail uploads of pt3 specifically. The other 5 chunks must still
	// upload successfully — sibling failures cannot cancel them.
	fc := &flakyClient{fakeClient: &fakeClient{}, failOnTitle: "(pt3)"}
	var buf bytes.Buffer
	err := Run(context.Background(), fc, "nb-flaky", []string{dir}, Options{Name: "test", MaxBytes: 2000, Parallel: 4}, &buf)
	if err == nil {
		t.Fatal("expected sync to surface the simulated failure")
	}
	if !strings.Contains(err.Error(), "simulated failure") {
		t.Errorf("error chain missing simulated failure: %v", err)
	}

	// Every other chunk should have landed; pt3 stays unuploaded.
	for _, u := range fc.uploaded {
		if strings.Contains(u.title, "(pt3)") {
			t.Errorf("flaky chunk %q should not have uploaded", u.title)
		}
	}
	if len(fc.uploaded) < 4 {
		t.Errorf("expected sibling chunks to complete despite pt3 failure; got %d uploads: %v", len(fc.uploaded), fc.uploaded)
	}
}

func TestRunParallelUploadsAllChunks(t *testing.T) {
	setupTestHome(t)
	dir := t.TempDir()
	// Create several smaller files; tight maxBytes pushes each into its
	// own chunk so the upload loop dispatches them in parallel.
	for i := range 6 {
		body := strings.Repeat(fmt.Sprintf("file-%d content line\n", i), 200)
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("f%d.txt", i)), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	fc := &fakeClient{}
	var buf bytes.Buffer
	err := Run(context.Background(), fc, "nb-par", []string{dir}, Options{Name: "test", MaxBytes: 5000, Parallel: 4}, &buf)
	if err != nil {
		t.Fatal(err)
	}
	if len(fc.uploaded) < 2 {
		t.Fatalf("expected multiple chunks uploaded, got %d", len(fc.uploaded))
	}
	titles := make(map[string]bool, len(fc.uploaded))
	for _, u := range fc.uploaded {
		if titles[u.title] {
			t.Errorf("duplicate upload title %q under parallel dispatch", u.title)
		}
		titles[u.title] = true
	}
}

func TestBundleSplitsOversizeSingleFile(t *testing.T) {
	dir := t.TempDir()
	// 30KB of line-structured content; bundle at 10KB max so this file
	// must split into multiple parts. Without splitting, sync would emit
	// one chunk that exceeds the per-request server limit.
	var b bytes.Buffer
	for i := 0; b.Len() < 30*1024; i++ {
		fmt.Fprintf(&b, "line %d: lorem ipsum dolor sit amet consectetur\n", i)
	}
	if err := os.WriteFile(filepath.Join(dir, "big.txt"), b.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	files, _ := walkFiles(dir)
	const maxBytes = 10 * 1024
	chunks, err := bundle(files, maxBytes, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) < 3 {
		t.Fatalf("expected file to split into >=3 chunks at %d-byte limit, got %d", maxBytes, len(chunks))
	}

	var reassembled bytes.Buffer
	for i, chunk := range chunks {
		if len(chunk) > maxBytes {
			t.Errorf("chunk %d size %d exceeds maxBytes %d", i, len(chunk), maxBytes)
		}
		want := fmt.Sprintf("big.txt (part %d/%d)", i+1, len(chunks))
		if !bytes.Contains(chunk, []byte("-- "+want+" --")) {
			t.Errorf("chunk %d missing entry %q", i, want)
		}
		ar := txtar.Parse(chunk)
		if len(ar.Files) != 1 {
			t.Fatalf("chunk %d has %d files, want 1", i, len(ar.Files))
		}
		reassembled.Write(ar.Files[0].Data)
	}
	if !bytes.Equal(reassembled.Bytes(), b.Bytes()) {
		t.Errorf("reassembled content does not match original (got %d bytes, want %d)", reassembled.Len(), b.Len())
	}
}

func TestBundleQuotesNestedTxtar(t *testing.T) {
	dir := t.TempDir()
	// File whose contents contain a txtar marker line — without quoting
	// this would split the outer archive into three phantom files.
	body := "top line\n-- trap.go --\npackage trap\n"
	os.WriteFile(filepath.Join(dir, "nested.md"), []byte(body), 0o644)

	files, err := gitFiles(dir, false)
	if err != nil || len(files) == 0 {
		files, _ = walkFiles(dir)
	}
	chunks, err := bundle(files, 5<<20, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	chunk := chunks[0]

	if !bytes.Contains(chunk, []byte("unquote ")) {
		t.Errorf("expected unquote directive in comment, chunk: %q", chunk)
	}
	// Raw nested marker must not appear as a bare line.
	if bytes.Contains(chunk, []byte("\n-- trap.go --\n")) {
		t.Errorf("unquoted marker leaked through: %q", chunk)
	}
	// Quoted form should appear.
	if !bytes.Contains(chunk, []byte(">-- trap.go --")) {
		t.Errorf("expected quoted marker in chunk: %q", chunk)
	}
}

// TestBundlePreProcessTransformsContent locks in the --pre-process contract:
// each file's bytes are piped through `sh -c cmd` and the command's stdout
// replaces what gets bundled. The original file name must reach the entry
// (not the command output's pseudo-name) and $NLM_FILE_NAME must carry the
// file name into the command's environment.
func TestBundlePreProcessTransformsContent(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "lower.txt"), []byte("hello world\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	files, _ := walkFiles(dir)
	chunks, err := bundle(files, 1<<20, `printf 'name=%s\n' "$NLM_FILE_NAME"; tr a-z A-Z`)
	if err != nil {
		t.Fatalf("bundle: %v", err)
	}
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	ar := txtar.Parse(chunks[0])
	if len(ar.Files) != 1 {
		t.Fatalf("expected 1 file in archive, got %d", len(ar.Files))
	}
	if ar.Files[0].Name != "lower.txt" {
		t.Errorf("entry name = %q, want lower.txt", ar.Files[0].Name)
	}
	got := string(ar.Files[0].Data)
	if !strings.Contains(got, "name=lower.txt") {
		t.Errorf("bundled content missing $NLM_FILE_NAME signal: %q", got)
	}
	if !strings.Contains(got, "HELLO WORLD") {
		t.Errorf("bundled content missing transformed body: %q", got)
	}
	// Raw original must be gone — the preprocess output replaces it.
	if strings.Contains(got, "hello world") {
		t.Errorf("bundled content still has untransformed body: %q", got)
	}
}

// TestBundlePreProcessNonZeroExitAborts locks in the abort-on-failure
// contract: a non-zero exit must stop the sync and surface the command's
// stderr in the error message so users can debug.
func TestBundlePreProcessNonZeroExitAborts(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "any.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	files, _ := walkFiles(dir)
	_, err := bundle(files, 1<<20, `echo "boom" >&2; exit 1`)
	if err == nil {
		t.Fatalf("expected error from failing pre-process, got nil")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("error %q does not surface stderr", err)
	}
}

func TestQuoteRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		{"marker only", "-- foo --\n"},
		{"marker mid", "hello\n-- foo --\nworld\n"},
		{"leading newline", "\n-- foo --\n"},
		{"no marker", "just text\nmore text\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := quote([]byte(tt.in))
			if err != nil {
				t.Fatalf("quote: %v", err)
			}
			u, err := unquote(q)
			if err != nil {
				t.Fatalf("unquote: %v", err)
			}
			if string(u) != tt.in {
				t.Errorf("round trip mismatch: got %q want %q", u, tt.in)
			}
		})
	}
}

func TestNeedsQuote(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"hello\nworld\n", false},
		{"hello\n-- foo --\n", true},
		{"-- foo --\n", true},
		{"not-- foo --\n", false},
	}
	for _, tt := range tests {
		if got := needsQuote([]byte(tt.in)); got != tt.want {
			t.Errorf("needsQuote(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestResolveName(t *testing.T) {
	dir := t.TempDir()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd: %v", err)
	}
	tests := []struct {
		name    string
		paths   []string
		want    string
		wantErr bool
	}{
		{"explicit", []string{"/any"}, "explicit", false},
		{"", []string{dir}, filepath.Base(dir), false},
		{"", []string{"/a", "/b"}, "", true},
		{"", []string{"."}, filepath.Base(cwd), false},
	}
	for _, tt := range tests {
		got, err := resolveName(tt.name, tt.paths)
		if (err != nil) != tt.wantErr {
			t.Errorf("resolveName(%q, %v) err=%v, wantErr=%v", tt.name, tt.paths, err, tt.wantErr)
			continue
		}
		if got != tt.want {
			t.Errorf("resolveName(%q, %v) = %q, want %q", tt.name, tt.paths, got, tt.want)
		}
	}
}
