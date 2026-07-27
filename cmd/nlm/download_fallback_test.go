package main

import (
	"errors"
	"io"
	"os"
	"strings"
	"testing"
)

func TestAudioDownloadFallbackPrintsBrowserURL(t *testing.T) {
	out, errText, err := captureStdoutStderr(t, func() error {
		return audioDownloadUnavailableError("notebook-123", errors.New("audio overview data not found"))
	})
	if err == nil {
		t.Fatal("audioDownloadUnavailableError unexpectedly succeeded")
	}
	if !strings.Contains(out, "https://notebook.google.com/notebook/notebook-123") {
		t.Fatalf("stdout did not include browser URL\nstdout:\n%s\nstderr:\n%s", out, errText)
	}
	if !strings.Contains(errText, "Open https://notebook.google.com/notebook/notebook-123 in a browser") {
		t.Fatalf("stderr did not include browser instruction\nstdout:\n%s\nstderr:\n%s", out, errText)
	}
	if !strings.Contains(err.Error(), "audio overview data not found") {
		t.Fatalf("error did not preserve cause: %v", err)
	}
}

func captureStdoutStderr(t *testing.T, fn func() error) (string, string, error) {
	t.Helper()

	oldStdout, oldStderr := os.Stdout, os.Stderr
	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stdout pipe: %v", err)
	}
	stderrR, stderrW, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stderr pipe: %v", err)
	}
	defer stdoutR.Close()
	defer stderrR.Close()

	os.Stdout = stdoutW
	os.Stderr = stderrW
	fnErr := fn()
	stdoutW.Close()
	stderrW.Close()
	os.Stdout = oldStdout
	os.Stderr = oldStderr

	stdout, err := io.ReadAll(stdoutR)
	if err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	stderr, err := io.ReadAll(stderrR)
	if err != nil {
		t.Fatalf("read stderr: %v", err)
	}
	return string(stdout), string(stderr), fnErr
}
