package nlmsync

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

// labelClient records ordered mutations and models live remote identity.
// This lets tests assert label transfer before the donor disappears.
type labelClient struct {
	mu              sync.Mutex
	sources         []Source
	labels          map[string][]string
	operations      []string
	next            int
	reads           int
	readErr         error
	attachFail      int
	attachAttempts  int
	dropAttachments bool
	cancel          context.CancelFunc
}

func (c *labelClient) ListSources(ctx context.Context, _ string) ([]Source, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]Source(nil), c.sources...), ctx.Err()
}
func (c *labelClient) LabelsForSource(ctx context.Context, _, id string) ([]string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.reads++
	return append([]string(nil), c.labels[id]...), errors.Join(ctx.Err(), c.readErr)
}
func (c *labelClient) AddSource(ctx context.Context, _, name string, r io.Reader) (string, error) {
	if _, err := io.Copy(io.Discard, r); err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.next++
	id := fmt.Sprintf("new-%d", c.next)
	c.sources = append(c.sources, Source{ID: id, Title: name})
	c.operations = append(c.operations, "add "+id+" "+name)
	return id, nil
}
func (c *labelClient) RenameSource(ctx context.Context, id, name string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := range c.sources {
		if c.sources[i].ID == id {
			c.sources[i].Title = name
		}
	}
	c.operations = append(c.operations, "rename "+id+" "+name)
	return nil
}
func (c *labelClient) AttachLabelSource(ctx context.Context, _, label, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.attachAttempts++
	if c.attachAttempts == c.attachFail {
		return fmt.Errorf("attach failed")
	}
	if !c.dropAttachments {
		c.labels[id] = unionLabels(c.labels[id], []string{label})
	}
	c.operations = append(c.operations, "attach "+id+" "+label)
	if c.cancel != nil {
		c.cancel()
	}
	return nil
}
func (c *labelClient) DeleteSources(ctx context.Context, _ string, ids []string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, id := range ids {
		for i := 0; i < len(c.sources); i++ {
			if c.sources[i].ID == id {
				c.sources = append(c.sources[:i], c.sources[i+1:]...)
				i--
			}
		}
		delete(c.labels, id)
		c.operations = append(c.operations, "delete "+id)
	}
	return nil
}
func (c *labelClient) labelsAt(title string) []string {
	for _, s := range c.sources {
		if s.Title == title {
			return c.labels[s.ID]
		}
	}
	return nil
}
func labelInput(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "a.txt")
	if err := os.WriteFile(path, []byte("content\n"), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}
func cacheLabelInput(t *testing.T, path string, opts Options) {
	t.Helper()
	chunks, names, err := Pack([]string{path}, opts)
	if err != nil {
		t.Fatal(err)
	}
	for i, n := range names {
		if err := newHashCache("nb").save(n, fmt.Sprintf("%x", sha256.Sum256(chunks[i]))); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLabelCapability(t *testing.T) {
	for _, dry := range []bool{false, true} {
		for _, kind := range []string{"unknown", "empty", "read failure", "opt out absent", "opt out present"} {
			t.Run(fmt.Sprint(dry)+kind, func(t *testing.T) {
				setupTestHome(t)
				path := labelInput(t)
				client := &labelClient{sources: []Source{{ID: "old", Title: "test"}}, labels: map[string][]string{}}
				var c Client = client
				opts := Options{Name: "test", DryRun: dry, JSON: true}
				switch kind {
				case "unknown":
					c = struct{ Client }{client}
				case "read failure":
					client.readErr = fmt.Errorf("label read failed")
				case "opt out absent":
					c = struct{ Client }{client}
					opts.NoLabels = true
				case "opt out present":
					opts.NoLabels = true
					client.readErr = fmt.Errorf("must not read")
				}
				err := Run(context.Background(), c, "nb", []string{path}, opts, io.Discard)
				fail := kind == "unknown" || kind == "read failure"
				if (err != nil) != fail {
					t.Fatalf("error=%v", err)
				}
				if (dry || fail) && len(client.operations) > 0 {
					t.Fatalf("mutations=%v", client.operations)
				}
				if opts.NoLabels && client.reads != 0 {
					t.Fatal("opt out read labels")
				}
			})
		}
	}
}

func TestPartLabelsAreAuthoritative(t *testing.T) {
	setupTestHome(t)
	sources := []Source{{ID: "parent", Title: "test"}, {ID: "a", Title: "test (pt1) (a)"}, {ID: "b", Title: "test (pt1) (b)"}}
	c := &labelClient{sources: sources, labels: map[string][]string{"parent": {"parent"}, "a": {"A"}}}
	plan, err := planLabels(context.Background(), c, "nb", "test", []string{"test"}, sources, Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name            string
		inherited, want []string
	}{
		{"test (pt1) (a)", []string{"parent"}, []string{"A"}},
		{"test (pt1) (b)", []string{"parent"}, nil},
		{"test (pt1) (aa)", []string{"A"}, []string{"A"}},
		{"test (pt1) (ba)", nil, nil},
	} {
		got := plan.labels(tt.name, tt.inherited, false)
		if !reflect.DeepEqual(got, tt.want) {
			t.Fatalf("%s=%v want %v", tt.name, got, tt.want)
		}
	}
	// A missing parent cannot erase an existing labeled leaf or give a new
	// sibling the family union.
	delete(plan.byTitle, "test")
	if got := plan.labels("test (pt1) (a)", nil, false); !reflect.DeepEqual(got, []string{"A"}) {
		t.Fatalf("existing leaf=%v", got)
	}
	if got := plan.labels("test (pt1) (ab)", nil, false); len(got) != 0 {
		t.Fatalf("new leaf=%v", got)
	}
}

func TestRecoveryLabelDonors(t *testing.T) {
	for _, mode := range []string{"success", "partial retry", "failure", "verify failure", "cancel", "old alone", "dry"} {
		t.Run(mode, func(t *testing.T) {
			setupTestHome(t)
			path := labelInput(t)
			opts := Options{Name: "test", JSON: true}
			cacheLabelInput(t, path, opts)
			c := &labelClient{sources: []Source{{ID: "new", Title: "test"}, {ID: "old", Title: "test [old]"}}, labels: map[string][]string{"old": {"A", "B"}}}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch mode {
			case "partial retry":
				c.attachFail = 2
			case "failure":
				c.attachFail = 1
			case "verify failure":
				c.dropAttachments = true
			case "cancel":
				c.cancel = cancel
			case "old alone":
				c.sources = c.sources[1:]
			case "dry":
				opts.DryRun = true
			}
			var out bytes.Buffer
			err := Run(ctx, c, "nb", []string{path}, opts, &out)
			fail := mode == "partial retry" || mode == "failure" || mode == "verify failure" || mode == "cancel"
			if (err != nil) != fail {
				t.Fatalf("error=%v", err)
			}
			if fail {
				for _, op := range c.operations {
					if op == "delete old" {
						t.Fatalf("deleted donor: %v", c.operations)
					}
				}
				if mode != "partial retry" {
					return
				}
				c.attachFail = 0
				if err := Run(context.Background(), c, "nb", []string{path}, opts, io.Discard); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "dry" {
				if len(c.operations) != 0 {
					t.Fatalf("dry mutations=%v", c.operations)
				}
				if !strings.Contains(out.String(), `"label_id":"A"`) || !strings.Contains(out.String(), `"source_id":"new"`) {
					t.Fatalf("plan=%s", out.String())
				}
				return
			}
			if !reflect.DeepEqual(c.labelsAt("test"), []string{"A", "B"}) {
				t.Fatalf("labels=%v", c.labels)
			}
			if mode == "old alone" {
				if !reflect.DeepEqual(c.operations, []string{"rename old test"}) {
					t.Fatalf("operations=%v", c.operations)
				}
				return
			}
			want := []string{"attach new A", "attach new B", "delete old"}
			if !reflect.DeepEqual(c.operations, want) {
				t.Fatalf("operations=%v want %v", c.operations, want)
			}
		})
	}
}

func TestSubtreeCollapseLabels(t *testing.T) {
	for _, auto := range []bool{false, true} {
		for _, dry := range []bool{false, true} {
			t.Run(fmt.Sprint(auto, dry), func(t *testing.T) {
				setupTestHome(t)
				path := labelInput(t)
				c := &labelClient{sources: []Source{{ID: "a", Title: "test"}, {ID: "b", Title: "test (pt2)"}, {ID: "lookalike", Title: "test (pt0)"}}, labels: map[string][]string{"a": {"A"}, "b": {"B"}, "lookalike": {"foreign"}}}
				var out bytes.Buffer
				err := Run(context.Background(), c, "nb", []string{path}, Options{Name: "test", AutoSplit: auto, DryRun: dry, JSON: true}, &out)
				if err != nil {
					t.Fatal(err)
				}
				if dry {
					if len(c.operations) != 0 {
						t.Fatalf("dry mutations=%v", c.operations)
					}
					dec := json.NewDecoder(&out)
					labels := 0
					for dec.More() {
						var e event
						if err := dec.Decode(&e); err != nil {
							t.Fatal(err)
						}
						if e.Action == "label" {
							labels++
							if !e.DryRun || e.Name != "test" || e.SourceID != "" {
								t.Fatalf("planned target=%+v", e)
							}
						}
					}
					if labels != 2 {
						t.Fatalf("label events=%d", labels)
					}
					return
				}
				if !reflect.DeepEqual(c.labelsAt("test"), []string{"A", "B"}) {
					t.Fatalf("labels=%v", c.labels)
				}
				if !reflect.DeepEqual(c.labels["lookalike"], []string{"foreign"}) {
					t.Fatal("unrelated source touched")
				}
				attachB, deleteB := -1, -1
				for i, op := range c.operations {
					if strings.HasSuffix(op, " B") {
						attachB = i
					}
					if op == "delete b" {
						deleteB = i
					}
				}
				if attachB < 0 || deleteB <= attachB {
					t.Fatalf("donor ordering=%v", c.operations)
				}
				c.operations = nil
				c.attachAttempts = 0
				if err := Run(context.Background(), c, "nb", []string{path}, Options{Name: "test", AutoSplit: auto}, io.Discard); err != nil {
					t.Fatal(err)
				}
				if len(c.operations) != 0 || c.attachAttempts != 0 {
					t.Fatalf("unchanged operations=%v", c.operations)
				}
			})
		}
	}
}

func TestAmbiguousRechunkRepeatsWithoutMutation(t *testing.T) {
	setupTestHome(t)
	path := labelInput(t)
	if err := os.WriteFile(path, []byte(strings.Repeat("data\n", 500)), 0600); err != nil {
		t.Fatal(err)
	}
	c := &labelClient{sources: []Source{{ID: "a", Title: "test"}}, labels: map[string][]string{"a": {"A"}}}
	for _, dry := range []bool{false, true} {
		for i := 0; i < 2; i++ {
			err := Run(context.Background(), c, "nb", []string{path}, Options{Name: "test", MaxBytes: 400, Force: true, DryRun: dry}, io.Discard)
			if err == nil || !strings.Contains(err.Error(), "ambiguous labeled rechunk") {
				t.Fatalf("error=%v", err)
			}
			if len(c.operations) > 0 {
				t.Fatalf("mutations=%v", c.operations)
			}
		}
	}
	c.labels["a"] = nil
	if err := Run(context.Background(), c, "nb", []string{path}, Options{Name: "test", MaxBytes: 400}, io.Discard); err != nil {
		t.Fatalf("unlabeled rechunk: %v", err)
	}
}

func TestSyncKeepsSiblingLabelsSeparate(t *testing.T) {
	for _, auto := range []bool{false, true} {
		t.Run(fmt.Sprint(auto), func(t *testing.T) {
			setupTestHome(t)
			path := labelInput(t)
			if err := os.WriteFile(path, []byte(strings.Repeat("content line\n", 500)), 0600); err != nil {
				t.Fatal(err)
			}
			opts := Options{Name: "test", AutoSplit: auto, MaxBytes: 100000, Parallel: 3}
			var sources []Source
			names := []string{"test (pt1) (a)", "test (pt1) (b)"}
			if !auto {
				opts.MaxBytes = 4000
				_, names0, err := Pack([]string{path}, opts)
				if err != nil {
					t.Fatal(err)
				}
				names = names0
			}
			for i, name := range names {
				sources = append(sources, Source{ID: fmt.Sprint(i), Title: name})
			}
			c := &labelClient{sources: sources, labels: map[string][]string{"0": {"A"}}}
			if err := Run(context.Background(), c, "nb", []string{path}, opts, io.Discard); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(c.labelsAt(names[0]), []string{"A"}) {
				t.Fatalf("first leaf=%v", c.labelsAt(names[0]))
			}
			for _, name := range names[1:] {
				if len(c.labelsAt(name)) != 0 {
					t.Fatalf("label bleed: %s=%v", name, c.labelsAt(name))
				}
			}
		})
	}
}

func TestNestedRetryWithoutParentLabels(t *testing.T) {
	setupTestHome(t)
	path := labelInput(t)
	if err := os.WriteFile(path, []byte(strings.Repeat("line\n", 3200)), 0600); err != nil {
		t.Fatal(err)
	}
	c := &labelClient{sources: []Source{{ID: "aa", Title: "test (pt1) (aa)"}, {ID: "b", Title: "test (pt1) (b)"}}, labels: map[string][]string{"aa": {"A"}, "b": {"B"}}}
	if err := Run(context.Background(), c, "nb", []string{path}, Options{Name: "test", AutoSplit: true, Parallel: 3}, io.Discard); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(c.labelsAt("test (pt1) (aa)"), []string{"A"}) {
		t.Fatalf("existing leaf=%v", c.labels)
	}
	if !reflect.DeepEqual(c.labelsAt("test (pt1) (b)"), []string{"B"}) {
		t.Fatalf("sibling=%v", c.labels)
	}
	found := false
	for _, s := range c.sources {
		if s.Title == "test (pt1) (ab)" {
			found = true
			if len(c.labels[s.ID]) != 0 {
				t.Fatalf("new leaf inherited sibling labels: %v", c.labels[s.ID])
			}
		}
	}
	if !found {
		t.Fatalf("missing new descendant: %v", c.sources)
	}
}

func TestCanonicalDuplicatesStopBeforeMutation(t *testing.T) {
	setupTestHome(t)
	path := labelInput(t)
	c := &labelClient{sources: []Source{{ID: "bare", Title: "test"}, {ID: "alias", Title: "test (pt1)"}}, labels: map[string][]string{}}
	if err := Run(context.Background(), c, "nb", []string{path}, Options{Name: "test"}, io.Discard); err == nil {
		t.Fatal("duplicate accepted")
	}
	if len(c.operations) != 0 {
		t.Fatalf("mutations=%v", c.operations)
	}
}

func TestLiteralBaseSync(t *testing.T) {
	for _, base := range []string{"notes (draft)", "notes (pt2)"} {
		t.Run(base, func(t *testing.T) {
			setupTestHome(t)
			path := labelInput(t)
			c := &labelClient{sources: []Source{{ID: "old", Title: base + " (pt1)"}, {ID: "foreign", Title: base + " (z)"}}, labels: map[string][]string{"old": {"A"}, "foreign": {"B"}}}
			if err := Run(context.Background(), c, "nb", []string{path}, Options{Name: base, AutoSplit: true}, io.Discard); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(c.labelsAt(base), []string{"A"}) || !reflect.DeepEqual(c.labelsAt(base+" (z)"), []string{"B"}) {
				t.Fatalf("sources=%v labels=%v", c.sources, c.labels)
			}
		})
	}
}

// TestSeedLabels covers Options.Labels: every part of the family carries the
// seed, including parts that inherit nothing and parts minted by a split,
// and the seed never displaces a part's own labels.
func TestSeedLabels(t *testing.T) {
	setupTestHome(t)
	sources := []Source{{ID: "parent", Title: "test"}, {ID: "a", Title: "test (pt1) (a)"}}
	c := &labelClient{sources: sources, labels: map[string][]string{"a": {"A"}}}
	opts := Options{Labels: []string{"seed"}}
	plan, err := planLabels(context.Background(), c, "nb", "test", []string{"test"}, sources, opts)
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name            string
		inherited, want []string
	}{
		{"test (pt1) (a)", nil, []string{"A", "seed"}},
		{"test (pt1) (b)", nil, []string{"seed"}},
		{"test (pt1) (b) (c)", []string{"X"}, []string{"X", "seed"}},
	} {
		if got := plan.labels(tt.name, tt.inherited, false); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s=%v want %v", tt.name, got, tt.want)
		}
	}
	if got := plan.labels("test", nil, true); !reflect.DeepEqual(got, []string{"A", "seed"}) {
		t.Errorf("collapsed=%v", got)
	}
}

func TestSeedLabelsRejectedWithNoLabels(t *testing.T) {
	setupTestHome(t)
	c := &labelClient{}
	_, err := planLabels(context.Background(), c, "nb", "test", []string{"test"}, nil, Options{NoLabels: true, Labels: []string{"seed"}})
	if err == nil {
		t.Fatal("planLabels(NoLabels+Labels) = nil error")
	}
}
