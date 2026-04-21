package sync

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupTestHome(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
}

type fakeClient struct {
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
	f.listed++
	return f.sources, nil
}

func (f *fakeClient) AddSource(_ context.Context, _ string, title string, r io.Reader) (string, error) {
	data, _ := io.ReadAll(r)
	f.uploaded = append(f.uploaded, struct {
		title   string
		content string
	}{title, string(data)})
	id := "src-" + title
	f.sources = append(f.sources, Source{ID: id, Title: title})
	return id, nil
}

func (f *fakeClient) DeleteSources(_ context.Context, _ string, ids []string) error {
	f.deleted = append(f.deleted, ids...)
	return nil
}

func (f *fakeClient) RenameSource(_ context.Context, id string, title string) error {
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

func TestBundleQuotesNestedTxtar(t *testing.T) {
	dir := t.TempDir()
	// File whose contents contain a txtar marker line — without quoting
	// this would split the outer archive into three phantom files.
	body := "top line\n-- trap.go --\npackage trap\n"
	os.WriteFile(filepath.Join(dir, "nested.md"), []byte(body), 0o644)

	files, err := gitFiles(dir)
	if err != nil || len(files) == 0 {
		files, _ = walkFiles(dir)
	}
	chunks, err := bundle(files, 5<<20)
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
	tests := []struct {
		name    string
		paths   []string
		want    string
		wantErr bool
	}{
		{"explicit", []string{"/any"}, "explicit", false},
		{"", []string{dir}, filepath.Base(dir), false},
		{"", []string{"/a", "/b"}, "", true},
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
