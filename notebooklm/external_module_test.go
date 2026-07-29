package notebooklm_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestExternalModule(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate package source")
	}
	root := filepath.Dir(filepath.Dir(file))
	dir := t.TempDir()

	goMod := fmt.Sprintf(`module example.com/notebooklmconsumer

go 1.25.0

require github.com/tmc/nlm v0.0.0

replace github.com/tmc/nlm => %s
`, filepath.ToSlash(root))
	writeExternalModuleFile(t, filepath.Join(dir, "go.mod"), goMod)
	writeExternalModuleFile(t, filepath.Join(dir, "client_test.go"), `package consumer

import (
	"context"
	"net/http"
	"testing"

	"github.com/tmc/nlm/notebooklm"
)

func TestClientCompiles(t *testing.T) {
	client := notebooklm.New(
		notebooklm.Credentials{AuthToken: "token", Cookies: "cookies"},
		notebooklm.WithAuthUser("1"),
		notebooklm.WithHTTPClient(&http.Client{}),
		notebooklm.WithURLParams(map[string]string{"hl": "en"}),
	)
	if client == nil {
		t.Fatal("nil client")
	}
}

func listNotebooks(ctx context.Context, client *notebooklm.Client) ([]*notebooklm.Notebook, error) {
	return client.ListRecentlyViewedProjects(ctx)
}

var _ notebooklm.SlideDeckFormat = notebooklm.SlideDeckFormatDetailed
`)

	cmd := exec.Command("go", "test", "-mod=mod", ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("external go test: %v\n%s", err, output)
	}
}

func writeExternalModuleFile(t *testing.T, name, content string) {
	t.Helper()
	if err := os.WriteFile(name, []byte(strings.TrimSpace(content)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}
