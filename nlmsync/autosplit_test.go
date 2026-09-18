package nlmsync

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tmc/nlm/internal/batchexecute"
	"golang.org/x/tools/txtar"
)

type rejectingClient struct {
	rejectName string
	*fakeLabelClient
	limit int
	// Parts upload concurrently, so the fake's own bookkeeping needs a lock
	// of its own; the embedded fakeClient guards only its fields.
	mu       sync.Mutex
	failure  error
	attempts int
	content  map[string][]byte
}

func (c *rejectingClient) AddSource(ctx context.Context, nb, name string, r io.Reader) (string, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return "", err
	}
	c.mu.Lock()
	c.attempts++
	reject, limit, failure := name == c.rejectName, c.limit, c.failure
	c.mu.Unlock()
	if reject {
		return "", &batchexecute.APIError{HTTPStatus: 403}
	}
	if len(data) > limit {
		if failure != nil {
			return "", failure
		}
		return "", &batchexecute.APIError{HTTPStatus: 500}
	}
	id, err := c.fakeClient.AddSource(ctx, nb, name, bytes.NewReader(data))
	if err == nil {
		c.mu.Lock()
		c.content[id] = data
		c.mu.Unlock()
	}
	return id, err
}
func (c *rejectingClient) RenameSource(ctx context.Context, id, title string) error {
	for i := range c.sources {
		if c.sources[i].ID == id {
			c.sources[i].Title = title
		}
	}
	return c.fakeClient.RenameSource(ctx, id, title)
}
func (c *rejectingClient) DeleteSources(ctx context.Context, nb string, ids []string) error {
	for _, id := range ids {
		for i := 0; i < len(c.sources); i++ {
			if c.sources[i].ID == id {
				c.sources = append(c.sources[:i], c.sources[i+1:]...)
				i--
			}
		}
		delete(c.labelsBySource, id)
		delete(c.content, id)
	}
	return c.fakeClient.DeleteSources(ctx, nb, ids)
}
func newRejectingClient() *rejectingClient {
	return &rejectingClient{fakeLabelClient: &fakeLabelClient{fakeClient: &fakeClient{sources: []Source{{ID: "original", Title: "test"}}}, labelsBySource: map[string][]string{"original": {"label"}}}, limit: 5000, content: map[string][]byte{}}
}

func TestAutoSplitRun(t *testing.T) {
	setupTestHome(t)
	path := filepath.Join(t.TempDir(), "a.txt")
	raw := []byte(strings.Repeat("-- embedded --\ntext without final padding", 400))
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	c := newRejectingClient()
	opts := Options{Name: "test", AutoSplit: true, JSON: true}
	var out bytes.Buffer
	if err := Run(context.Background(), c, "nb", []string{path}, opts, &out); err != nil {
		t.Fatal(err)
	}
	if len(c.sources) < 2 || !strings.Contains(out.String(), `"action":"split"`) {
		t.Fatalf("no split: sources=%v output=%s", c.sources, &out)
	}
	// Parts upload concurrently, so the client's source order reflects
	// completion order; leaf titles sort into content order.
	parts := append([]Source(nil), c.sources...)
	sort.Slice(parts, func(i, j int) bool { return parts[i].Title < parts[j].Title })
	var restored []byte
	for _, s := range parts {
		if len(c.labelsBySource[s.ID]) != 1 {
			t.Fatalf("labels missing on %s", s.Title)
		}
		ar := txtar.Parse(c.content[s.ID])
		for i := range ar.Files {
			data, err := restoreArchiveFile(ar, i)
			if err != nil {
				t.Fatal(err)
			}
			restored = append(restored, data...)
		}
	}
	if !bytes.Equal(restored, raw) {
		t.Fatal("split changed source bytes")
	}
	attempts := c.attempts
	if err := Run(context.Background(), c, "nb", []string{path}, opts, io.Discard); err != nil {
		t.Fatal(err)
	}
	if c.attempts != attempts {
		t.Fatal("unchanged split parts uploaded again")
	}
	// Growing max-bytes or disabling auto-split can consolidate the family.
	c.limit = 100000
	opts.AutoSplit = false
	if err := Run(context.Background(), c, "nb", []string{path}, opts, io.Discard); err != nil {
		t.Fatal(err)
	}
	if len(c.sources) != 1 || c.sources[0].Title != "test" {
		t.Fatalf("stale split parts: %v", c.sources)
	}
}

func TestAutoSplitFailures(t *testing.T) {
	for _, tc := range []struct {
		name         string
		enabled      bool
		err          error
		limit        int
		wantAttempts int
	}{
		{"disabled", false, nil, 5000, uploadAttempts},
		{"authentication", true, &batchexecute.APIError{HTTPStatus: 401}, 5000, 1},
		{"rate limit", true, &batchexecute.APIError{HTTPStatus: 429}, 5000, 1},
		{"transport", true, fmt.Errorf("connection refused"), 5000, 1},
		{"floor", true, nil, 1, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setupTestHome(t)
			path := filepath.Join(t.TempDir(), "a.txt")
			if err := os.WriteFile(path, []byte(strings.Repeat("text\n", 3000)), 0600); err != nil {
				t.Fatal(err)
			}
			c := newRejectingClient()
			c.failure = tc.err
			c.limit = tc.limit
			err := Run(context.Background(), c, "nb", []string{path}, Options{Name: "test", AutoSplit: tc.enabled}, io.Discard)
			if err == nil {
				t.Fatal("expected failure")
			}
			if tc.wantAttempts > 0 && c.attempts != tc.wantAttempts {
				t.Fatalf("attempts=%d", c.attempts)
			}
			// Each level may also retry, so the bound covers the whole
			// tree of attempts, not one attempt per split.
			if c.attempts > 24 {
				t.Fatalf("unbounded splitting: %d", c.attempts)
			}
			if len(c.sources) != 1 || c.sources[0].ID != "original" || c.sources[0].Title != "test" || len(c.labelsBySource["original"]) != 1 {
				t.Fatalf("original lost: %v", c.sources)
			}
		})
	}
}

func TestAutoSplitDryRunAndCancellation(t *testing.T) {
	setupTestHome(t)
	path := filepath.Join(t.TempDir(), "a.txt")
	if err := os.WriteFile(path, []byte(strings.Repeat("text\n", 3000)), 0600); err != nil {
		t.Fatal(err)
	}
	c := newRejectingClient()
	if err := Run(context.Background(), c, "nb", []string{path}, Options{Name: "test", AutoSplit: true, DryRun: true}, io.Discard); err != nil {
		t.Fatal(err)
	}
	if c.attempts != 0 || len(c.renamed) != 0 || len(c.deleted) != 0 || len(c.attachCalls) != 0 {
		t.Fatal("dry run mutated sources")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Run(ctx, c, "nb", []string{path}, Options{Name: "test", AutoSplit: true}, io.Discard); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v", err)
	}
	if c.attempts != 0 {
		t.Fatal("uploaded after cancellation")
	}
}

// Code 9 is the one size-dependent case: the server returns it both for a
// notebook at the source cap, which splitting cannot cure, and for an archive
// it will not take whole, which halving does cure. Field measurement: a
// 4,829,241-byte part was rejected with code 9 and the same content uploaded
// as four parts under 1.5 MB.
func TestAutoSplitErrorCodes(t *testing.T) {
	for _, tc := range []struct {
		code int
		size int
		want bool
	}{
		{7, 8 << 20, false},
		{8, 8 << 20, false},
		{9, 4829241, true},
		{9, 64 << 10, false},
		{13, 8 << 20, true},
		{14, 8 << 20, true},
		{16, 8 << 20, false},
	} {
		t.Run(fmt.Sprintf("%d/%d", tc.code, tc.size), func(t *testing.T) {
			code, _ := batchexecute.GetErrorCode(tc.code)
			err := &uploadError{err: &batchexecute.APIError{ErrorCode: code}}
			if got := canSplit(err, tc.size); got != tc.want {
				t.Fatalf("canSplit = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestAutoSplitPartialRetry(t *testing.T) {
	setupTestHome(t)
	path := filepath.Join(t.TempDir(), "a.txt")
	if err := os.WriteFile(path, []byte(strings.Repeat("text\n", 3000)), 0600); err != nil {
		t.Fatal(err)
	}
	c := newRejectingClient()
	c.rejectName = "test (pt1) (b)"
	opts := Options{Name: "test", AutoSplit: true}
	if err := Run(context.Background(), c, "nb", []string{path}, opts, io.Discard); err == nil {
		t.Fatal("expected failure")
	}
	found := false
	for _, s := range c.sources {
		if s.ID == "original" {
			found = true
		}
	}
	if !found || len(c.labelsBySource["original"]) == 0 {
		t.Fatal("lost original after partial failure")
	}
	c.rejectName = ""
	if err := Run(context.Background(), c, "nb", []string{path}, opts, io.Discard); err != nil {
		t.Fatal(err)
	}
	for _, s := range c.sources {
		if s.ID == "original" {
			t.Fatal("left obsolete parent")
		}
		if len(c.labelsBySource[s.ID]) == 0 {
			t.Fatal("missing label")
		}
	}
}

func TestRunRecoversAbandonedReplacementLabels(t *testing.T) {
	for _, auto := range []bool{false, true} {
		t.Run(fmt.Sprint(auto), func(t *testing.T) {
			setupTestHome(t)
			path := filepath.Join(t.TempDir(), "a.txt")
			if err := os.WriteFile(path, []byte("new content"), 0600); err != nil {
				t.Fatal(err)
			}
			c := newRejectingClient()
			c.sources[0].Title = "test [old]"
			if err := Run(context.Background(), c, "nb", []string{path}, Options{Name: "test", AutoSplit: auto}, io.Discard); err != nil {
				t.Fatal(err)
			}
			if len(c.sources) != 1 || c.sources[0].Title != "test" || len(c.labelsBySource[c.sources[0].ID]) == 0 {
				t.Fatalf("recovery: %v labels=%v", c.sources, c.labelsBySource)
			}
		})
	}
}

func TestAutoSplitNestedSerial(t *testing.T) {
	setupTestHome(t)
	path := filepath.Join(t.TempDir(), "a.txt")
	if err := os.WriteFile(path, []byte(strings.Repeat("text\n", 6000)), 0600); err != nil {
		t.Fatal(err)
	}
	c := newRejectingClient()
	c.failure = &batchexecute.APIError{HTTPStatus: 413}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := Run(ctx, c, "nb", []string{path}, Options{Name: "test", AutoSplit: true, Parallel: -1}, io.Discard); err != nil {
		t.Fatal(err)
	}
	if len(c.content) < 4 {
		t.Fatalf("got %d leaves, want nested splits", len(c.content))
	}
}

func TestBalancePoint(t *testing.T) {
	file := func(n int) txtar.File { return txtar.File{Data: make([]byte, n)} }
	for _, tt := range []struct {
		name  string
		files []txtar.File
		want  int
	}{
		{"even", []txtar.File{file(10), file(10), file(10), file(10)}, 2},
		{"dominant first", []txtar.File{file(100), file(1), file(1)}, 1},
		{"dominant last", []txtar.File{file(1), file(1), file(100)}, 2},
		{"two members", []txtar.File{file(1), file(100)}, 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := balancePoint(tt.files); got != tt.want {
				t.Errorf("balancePoint = %d, want %d", got, tt.want)
			}
		})
	}
}

// TestAutoSplitPreconditionRejection covers the field report that --auto-split
// did not rescue an upload the server rejected with code 9: canSplit read the
// code's non-retryable flag and gave up, so the whole family failed while the
// same content synced fine under a smaller --max-bytes. Parts at or above the
// floor now halve until they land.
func TestAutoSplitPreconditionRejection(t *testing.T) {
	setupTestHome(t)
	path := filepath.Join(t.TempDir(), "a.txt")
	// Large enough that the first two levels stay above the split floor.
	if err := os.WriteFile(path, []byte(strings.Repeat("iroh spec line\n", 400000)), 0600); err != nil {
		t.Fatal(err)
	}
	code, _ := batchexecute.GetErrorCode(9)
	c := newRejectingClient()
	c.failure = &batchexecute.APIError{ErrorCode: code}
	c.limit = 2 << 20
	opts := Options{Name: "test", AutoSplit: true, MaxBytes: 8 << 20}
	if err := Run(context.Background(), c, "nb", []string{path}, opts, io.Discard); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(c.sources) < 2 {
		t.Fatalf("sources = %d, want the family split into parts: %v", len(c.sources), c.sources)
	}
	for _, s := range c.sources {
		if len(c.content[s.ID]) > c.limit {
			t.Fatalf("part %q is %d bytes, above the server limit %d", s.Title, len(c.content[s.ID]), c.limit)
		}
	}
}
