package nlmsync

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func ExampleIncompleteError() {
	err := &IncompleteError{Name: "manual", Receipt: "attempt.json", Err: io.ErrUnexpectedEOF}
	fmt.Println(errors.Is(err, io.ErrUnexpectedEOF))
	// Output: true
}

func readResult(t *testing.T, data []byte) syncReceipt {
	t.Helper()
	var result syncReceipt
	dec := json.NewDecoder(bytes.NewReader(data))
	for {
		var r syncReceipt
		if err := dec.Decode(&r); err == io.EOF {
			break
		} else if err != nil {
			t.Fatal(err)
		}
		if result.Action == "result" {
			t.Fatal("event after final result")
		}
		if r.Action == "result" {
			result = r
		}
	}
	if result.Action != "result" {
		t.Fatal("missing final result")
	}
	return result
}

func TestSyncReceiptPartialFailure(t *testing.T) {
	setupTestHome(t)
	dir := t.TempDir()
	for i := range 3 {
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("%d.txt", i)), []byte(strings.Repeat("line\n", 200)), 0600); err != nil {
			t.Fatal(err)
		}
	}
	c := &flakyClient{fakeClient: &fakeClient{}, failOnTitle: "(pt2)"}
	var out bytes.Buffer
	err := Run(context.Background(), c, "nb", []string{dir}, Options{Name: "test", MaxBytes: 1500, JSON: true}, &out)
	var incomplete *IncompleteError
	if !errors.As(err, &incomplete) {
		t.Fatalf("error = %v, want IncompleteError", err)
	}
	r := readResult(t, out.Bytes())
	if r.Status != "incomplete" || r.CleanupComplete || r.Readiness != "unchecked" || len(r.Parts) != 3 {
		t.Fatalf("receipt = %+v", r)
	}
	var uploads, failures int
	for _, op := range r.Operations {
		switch op.Action {
		case "upload":
			uploads++
		case "error":
			failures++
			if op.Name != "test (pt2)" || op.Bytes != r.Parts[1].Bytes || !strings.Contains(op.Reason, fmt.Sprintf("(%d bytes)", op.Bytes)) {
				t.Fatalf("failed upload = %+v", op)
			}
		}
	}
	if uploads != 2 || failures != 1 {
		t.Fatalf("uploads=%d failures=%d", uploads, failures)
	}
	data, err := os.ReadFile(incomplete.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	if got := readResult(t, data); got.Status != r.Status || len(got.Operations) != len(r.Operations) {
		t.Fatalf("disk receipt differs: %+v", got)
	}
}

func TestSyncReceiptDamagedFamily(t *testing.T) {
	for _, auto := range []bool{false, true} {
		t.Run(fmt.Sprint(auto), func(t *testing.T) {
			setupTestHome(t)
			path := filepath.Join(t.TempDir(), "a.txt")
			if err := os.WriteFile(path, []byte("text"), 0600); err != nil {
				t.Fatal(err)
			}
			c := &fakeClient{sources: []Source{{ID: "good", Title: "test"}, {ID: "bad", Title: "test", Status: "error"}}}
			var out bytes.Buffer
			err := Run(context.Background(), c, "nb", []string{path}, Options{Name: "test", AutoSplit: auto, JSON: true}, &out)
			if err == nil || !strings.Contains(err.Error(), "duplicate part") || !strings.Contains(err.Error(), "status error") {
				t.Fatalf("error = %v", err)
			}
			if len(c.uploaded)+len(c.renamed)+len(c.deleted) != 0 {
				t.Fatal("mutated damaged family")
			}
			if r := readResult(t, out.Bytes()); r.Status != "incomplete" || r.CleanupComplete {
				t.Fatalf("receipt = %+v", r)
			}
		})
	}
}

func TestSyncReceiptAttemptIsolation(t *testing.T) {
	setupTestHome(t)
	newAttempt := func(opts Options) *syncReceipt {
		r, err := newSyncReceipt("nb", "test", []string{"test"}, []string{"hash"}, [][]byte{[]byte("text")}, opts)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	a, b := newAttempt(Options{}), newAttempt(Options{})
	if a.Path == b.Path {
		t.Fatal("attempts share a receipt")
	}
	for _, r := range []*syncReceipt{a, b} {
		data, err := os.ReadFile(r.Path)
		if err != nil {
			t.Fatal(err)
		}
		if got := readResult(t, data); got.Status != "in-progress" || got.CleanupComplete {
			t.Fatalf("premature success: %+v", got)
		}
	}
	dry := newAttempt(Options{DryRun: true})
	var out bytes.Buffer
	if err := (&outputWriter{w: &out, json: true, receipt: dry}).finish(nil); err != nil {
		t.Fatal(err)
	}
	if r := readResult(t, out.Bytes()); r.Path != "" || r.Status != "planned" || r.CleanupComplete {
		t.Fatalf("dry-run receipt = %+v", r)
	}
}

func TestSyncReceiptWriteFailure(t *testing.T) {
	setupTestHome(t)
	r, err := newSyncReceipt("nb", "test", nil, nil, nil, Options{})
	if err != nil {
		t.Fatal(err)
	}
	// Break persistence after the initial in-progress receipt was written.
	if err := os.RemoveAll(filepath.Dir(r.Path)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Dir(r.Path), []byte("blocked"), 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	o := &outputWriter{w: &out, json: true, receipt: r}
	o.emit(event{Action: "upload", Name: "test", SourceID: "new"})
	if err := o.finish(nil); err == nil {
		t.Fatal("receipt write failure returned success")
	}
	if got := readResult(t, out.Bytes()); got.Status != "incomplete" {
		t.Fatalf("status = %q", got.Status)
	}
}

func TestSyncReceiptCannotStart(t *testing.T) {
	setupTestHome(t)
	if err := os.MkdirAll(cacheRoot(), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheRoot(), "sync-attempts"), []byte("blocked"), 0600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "a.txt")
	if err := os.WriteFile(path, []byte("text"), 0600); err != nil {
		t.Fatal(err)
	}
	c := &fakeClient{}
	if err := Run(context.Background(), c, "nb", []string{path}, Options{Name: "test"}, io.Discard); err == nil {
		t.Fatal("started sync without receipt")
	}
	if c.listed != 0 || len(c.uploaded)+len(c.renamed)+len(c.deleted) != 0 {
		t.Fatal("called server without receipt")
	}
}

type interruptedSyncClient struct{ *fakeClient }

func (c *interruptedSyncClient) ListSources(context.Context, string) ([]Source, error) {
	panic("interrupted")
}

func TestSyncReceiptInterrupted(t *testing.T) {
	setupTestHome(t)
	path := filepath.Join(t.TempDir(), "a.txt")
	if err := os.WriteFile(path, []byte("text"), 0600); err != nil {
		t.Fatal(err)
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Error("expected interruption")
			}
		}()
		Run(context.Background(), &interruptedSyncClient{&fakeClient{}}, "nb", []string{path}, Options{Name: "test"}, io.Discard)
	}()
	paths, err := filepath.Glob(filepath.Join(cacheRoot(), "sync-attempts", "*", "attempt-*.json"))
	if err != nil || len(paths) != 1 {
		t.Fatalf("receipts = %v, error = %v", paths, err)
	}
	data, err := os.ReadFile(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	if r := readResult(t, data); r.Status != "in-progress" || r.CleanupComplete {
		t.Fatalf("interrupted attempt reported completion: %+v", r)
	}
}
