package nlmsync

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

type overlapWriter struct {
	active  atomic.Int32
	overlap atomic.Bool
}

func (w *overlapWriter) Write(p []byte) (int, error) {
	if w.active.Add(1) != 1 {
		w.overlap.Store(true)
	}
	time.Sleep(20 * time.Millisecond)
	w.active.Add(-1)
	return len(p), nil
}

func TestRunSerializesProgress(t *testing.T) {
	setupTestHome(t)
	dir := t.TempDir()
	paths := []string{filepath.Join(dir, "a.txt"), filepath.Join(dir, "b.txt")}
	for _, p := range paths {
		if err := os.WriteFile(p, []byte("some content\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	chunks, names, err := Pack(paths, Options{Name: "test", MaxBytes: 150})
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != 2 {
		t.Fatalf("chunks=%d", len(chunks))
	}
	hc := newHashCache("nb")
	if err := hc.save(names[1], fmt.Sprintf("%x", sha256.Sum256(chunks[1]))); err != nil {
		t.Fatal(err)
	}
	c := &fakeClient{sources: []Source{{ID: "unchanged", Title: names[1]}}}
	var out overlapWriter
	if err := Run(context.Background(), c, "nb", paths, Options{Name: "test", MaxBytes: 150, JSON: true}, &out); err != nil {
		t.Fatal(err)
	}
	if out.overlap.Load() {
		t.Fatal("concurrent progress writes")
	}
}
