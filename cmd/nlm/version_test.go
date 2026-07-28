package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestVersionStringStamped(t *testing.T) {
	old := version
	version = "v1.2.3"
	t.Cleanup(func() {
		version = old
	})

	got := versionString()
	if want := "nlm v1.2.3"; got != want {
		t.Fatalf("versionString() = %q, want %q", got, want)
	}
}

func TestVersionLinkerFlag(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "nlm")
	if os.PathSeparator == '\\' {
		exe += ".exe"
	}
	cmd := exec.Command("go", "build", "-ldflags=-X main.version=v1.2.3", "-o", exe, ".")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	cmd = exec.Command(exe, "--version")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("nlm --version: %v\n%s", err, out)
	}
	if got, want := string(out), "nlm v1.2.3\n"; got != want {
		t.Fatalf("nlm --version = %q, want %q", got, want)
	}
}
