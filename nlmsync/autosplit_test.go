package nlmsync

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tmc/nlm/internal/batchexecute"
	"golang.org/x/tools/txtar"
)

type rejectingClient struct {
	rejectName string
	*fakeLabelClient
	limit    int
	failure  error
	attempts int
	content  map[string][]byte
}

func (c *rejectingClient) AddSource(ctx context.Context, nb, name string, r io.Reader) (string, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return "", err
	}
	c.attempts++
	if name == c.rejectName {
		return "", &batchexecute.APIError{HTTPStatus: 403}
	}
	if len(data) > c.limit {
		if c.failure != nil {
			return "", c.failure
		}
		return "", &batchexecute.APIError{HTTPStatus: 500}
	}
	id, err := c.fakeClient.AddSource(ctx, nb, name, bytes.NewReader(data))
	if err == nil {
		c.content[id] = data
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
	var restored []byte
	for _, s := range c.sources {
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
		{"disabled", false, nil, 5000, 1},
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
			if c.attempts > 10 {
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
	if err := Run(ctx, c, "nb", []string{path}, Options{Name: "test", AutoSplit: true}, io.Discard); err != context.Canceled {
		t.Fatalf("error = %v", err)
	}
	if c.attempts != 0 {
		t.Fatal("uploaded after cancellation")
	}
}

func TestAutoSplitErrorCodes(t *testing.T) {
	for _, tc := range []struct {
		code int
		want bool
	}{{7, false}, {8, false}, {9, false}, {13, true}, {14, true}, {16, false}} {
		t.Run(fmt.Sprint(tc.code), func(t *testing.T) {
			code, _ := batchexecute.GetErrorCode(tc.code)
			err := &uploadError{err: &batchexecute.APIError{ErrorCode: code}}
			if got := canSplit(err); got != tc.want {
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
	c.rejectName = "test (split2)"
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
