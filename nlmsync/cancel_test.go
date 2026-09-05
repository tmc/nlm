package nlmsync

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
)

type canceledUploadClient struct {
	*fakeClient
	cancel context.CancelFunc
}

func (c *canceledUploadClient) AddSource(ctx context.Context, nb, title string, r io.Reader) (string, error) {
	c.cancel()
	return "", ctx.Err()
}
func (c *canceledUploadClient) RenameSource(ctx context.Context, id, title string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return c.fakeClient.RenameSource(ctx, id, title)
}
func TestRunCancellationRestoresTitle(t *testing.T) {
	setupTestHome(t)
	path := filepath.Join(t.TempDir(), "a.txt")
	if err := os.WriteFile(path, []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := &canceledUploadClient{fakeClient: &fakeClient{sources: []Source{{ID: "old", Title: "test"}}}, cancel: cancel}
	if err := Run(ctx, c, "nb", []string{path}, Options{Name: "test"}, io.Discard); err == nil {
		t.Fatal("expected canceled upload")
	}
	if len(c.renamed) != 2 || c.renamed[1].title != "test" {
		t.Fatalf("old title not restored: %v", c.renamed)
	}
}
