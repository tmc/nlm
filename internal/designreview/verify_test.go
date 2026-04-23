package designreview

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtract(t *testing.T) {
	report := `Top smells:
- control_socket.go:255 mixes transport and VM lifecycle.
- Global state read in macos.go:724 is wrong because…
- Here is a code fence with a citation: ` + "`proxy.go:128`" + ` bundles exec.
- This is not a citation: http://host:80/path.
- Neither is this timestamp: 12:34:56.
- Section header 1. First item
- Another: ./path/to/file.go:12 inline.
`
	cites := Extract(strings.NewReader(report))

	want := []struct {
		file string
		line int
	}{
		{"control_socket.go", 255},
		{"macos.go", 724},
		{"proxy.go", 128},
		{"./path/to/file.go", 12},
	}
	if len(cites) != len(want) {
		t.Fatalf("got %d citations, want %d: %+v", len(cites), len(want), cites)
	}
	for i, w := range want {
		if cites[i].File != w.file || cites[i].Line != w.line {
			t.Errorf("cites[%d] = %s:%d, want %s:%d", i, cites[i].File, cites[i].Line, w.file, w.line)
		}
	}
}

// fixtureRepo builds a tiny tree so Verify has something to match against.
func fixtureRepo(t *testing.T) *Repo {
	t.Helper()
	root := t.TempDir()
	must := func(rel, body string) {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	// 50-line file.
	must("control_socket.go", strings.Repeat("// comment\n", 50))
	// 10-line file in a subdir.
	must("cmd/proxy.go", strings.Repeat("line\n", 10))
	// Second file with same basename in another subdir — ambiguous.
	must("internal/proxy.go", "a\nb\n")

	repo := &Repo{}
	if err := repo.Scan(root, nil); err != nil {
		t.Fatalf("Scan: %v", err)
	}
	return repo
}

func TestVerify_OK(t *testing.T) {
	repo := fixtureRepo(t)
	cites := Verify(repo, []Citation{{File: "control_socket.go", Line: 10}})
	if cites[0].Status != StatusOK {
		t.Errorf("status = %s, want ok", cites[0].Status)
	}
	if cites[0].Match != "control_socket.go" {
		t.Errorf("match = %s, want control_socket.go", cites[0].Match)
	}
}

func TestVerify_FileMissing(t *testing.T) {
	repo := fixtureRepo(t)
	cites := Verify(repo, []Citation{{File: "appkit_compat.go", Line: 68}})
	if cites[0].Status != StatusFileMiss {
		t.Errorf("status = %s, want file_miss", cites[0].Status)
	}
	if cites[0].Reason == "" {
		t.Errorf("reason empty")
	}
}

func TestVerify_LineBeyondEOF(t *testing.T) {
	repo := fixtureRepo(t)
	cites := Verify(repo, []Citation{{File: "control_socket.go", Line: 9999}})
	if cites[0].Status != StatusLineMiss {
		t.Errorf("status = %s, want line_miss", cites[0].Status)
	}
	if !strings.Contains(cites[0].Reason, "beyond EOF") {
		t.Errorf("reason = %q, want mention of EOF", cites[0].Reason)
	}
}

func TestVerify_AmbiguousBasename(t *testing.T) {
	repo := fixtureRepo(t)
	cites := Verify(repo, []Citation{{File: "proxy.go", Line: 1}})
	if cites[0].Status != StatusAmbiguous {
		t.Errorf("status = %s, want ambiguous", cites[0].Status)
	}
	if !strings.Contains(cites[0].Reason, "basename matches") {
		t.Errorf("reason = %q", cites[0].Reason)
	}
}

func TestVerify_EndToEndFromReport(t *testing.T) {
	// Covers the hardening-doc contract: given a mixed report, good citations
	// survive and invented or stale ones fail.
	repo := fixtureRepo(t)
	report := `Findings:
- control_socket.go:10 is real.
- appkit_compat.go:68 is invented.
- control_socket.go:9999 is stale.
- proxy.go:1 is ambiguous.
`
	cites := Verify(repo, Extract(strings.NewReader(report)))
	want := []Status{StatusOK, StatusFileMiss, StatusLineMiss, StatusAmbiguous}
	if len(cites) != len(want) {
		t.Fatalf("got %d cites, want %d: %+v", len(cites), len(want), cites)
	}
	for i, w := range want {
		if cites[i].Status != w {
			t.Errorf("cites[%d].Status = %s, want %s (%s)", i, cites[i].Status, w, cites[i].Raw)
		}
	}
}

func TestCountLinesSingleLineWithoutTrailingNewline(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "single.txt")
	if err := os.WriteFile(path, []byte("only line"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if got := countLines(path); got != 1 {
		t.Fatalf("countLines(single line) = %d, want 1", got)
	}
}

func TestVerify_EmptyFileRejectsLineOne(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "empty.txt")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	repo := &Repo{}
	if err := repo.Scan(root, nil); err != nil {
		t.Fatalf("Scan: %v", err)
	}
	cites := Verify(repo, []Citation{{File: "empty.txt", Line: 1}})
	if cites[0].Status != StatusLineMiss {
		t.Fatalf("status = %s, want %s", cites[0].Status, StatusLineMiss)
	}
}
