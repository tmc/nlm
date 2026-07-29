package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestParseDeckDownloadArgs(t *testing.T) {
	command, ok := lookupCommand("deck download")
	if !ok {
		t.Fatal("deck download command not found")
	}
	parsed, err := parseCommandSpec(command.spec, command.surfaceSpec, []string{
		"notebook-123",
		"--id", "artifact-456",
		"--format", "pptx",
		"--output", "deck.pptx",
	}, globalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	args, err := decodeDeckDownloadArgs(parsed)
	if err != nil {
		t.Fatal(err)
	}
	if args.NotebookID != "notebook-123" {
		t.Fatalf("notebookID = %q, want notebook-123", args.NotebookID)
	}
	if args.Options.ArtifactID != "artifact-456" || args.Options.Format != "pptx" || args.Options.Output != "deck.pptx" {
		t.Fatalf("options = %+v", args.Options)
	}
}

func TestParseDeckDownloadArgsRejectsBadFormat(t *testing.T) {
	command, ok := lookupCommand("deck download")
	if !ok {
		t.Fatal("deck download command not found")
	}
	parsed, err := parseCommandSpec(command.spec, command.surfaceSpec, []string{
		"notebook-123",
		"--id", "artifact-456",
		"--format", "keynote",
	}, globalOptions{})
	if err == nil {
		_, err = decodeDeckDownloadArgs(parsed)
	}
	if err == nil || !strings.Contains(err.Error(), "unsupported format") {
		t.Fatalf("parse deck download error = %v, want unsupported format", err)
	}
}

// TestDeckDownloadRequiresAuth verifies that deck download now performs a real
// authenticated fetch: without credentials it reports an auth error rather than
// silently succeeding. (Previously it was a no-auth command that only printed a
// browser URL — issue #31: --format pptx "Download failed".)
func TestDeckDownloadRequiresAuth(t *testing.T) {
	tmpHome, err := os.MkdirTemp("", "nlm-test-home-*")
	if err != nil {
		t.Fatalf("failed to create temp home: %v", err)
	}
	defer os.RemoveAll(tmpHome)

	cmd := exec.Command("./nlm_test", "deck", "download", "notebook-123", "--id", "artifact-456", "--format", "pptx")
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + tmpHome}
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("deck download unexpectedly succeeded without auth\n%s", output)
	}
	out := string(output)
	if !strings.Contains(out, "uthentication") {
		t.Fatalf("deck download without auth did not report an auth error\n%s", out)
	}
}

// TestLegacySlideDeckDownloadWarnsDeprecated verifies the legacy alias still
// prints the deprecation warning and routes to the same (now real) downloader.
func TestLegacySlideDeckDownloadWarnsDeprecated(t *testing.T) {
	tmpHome, err := os.MkdirTemp("", "nlm-test-home-*")
	if err != nil {
		t.Fatalf("failed to create temp home: %v", err)
	}
	defer os.RemoveAll(tmpHome)

	cmd := exec.Command("./nlm_test", "download", "slide-deck", "notebook-123", "--id", "artifact-456", "--format", "pptx", "--output", "deck.pptx")
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + tmpHome}
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("legacy download unexpectedly succeeded without auth\n%s", output)
	}
	out := string(output)
	if !strings.Contains(out, "nlm: 'download slide-deck' is deprecated; use 'deck download'") {
		t.Fatalf("legacy download did not print compatibility warning\n%s", out)
	}
}
