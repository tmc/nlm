package main

import (
	"bytes"
	"os"
	"strings"
	"syscall"
	"testing"
)

func TestWriteInterruptCleanupRestoresTerminal(t *testing.T) {
	var buf bytes.Buffer
	writeInterruptCleanup(&buf, os.Interrupt)
	got := buf.String()
	if !strings.HasPrefix(got, "\r\033[K"+ansiReset) {
		t.Fatalf("cleanup = %q, want a cleared line and an ANSI reset first", got)
	}
	if !strings.HasSuffix(got, "\n") {
		t.Fatalf("cleanup = %q, want a trailing newline", got)
	}
	if !strings.Contains(got, "interrupted") {
		t.Fatalf("cleanup = %q, want the interrupt notice", got)
	}
}

// Stopping the handler must be safe and idempotent-by-construction: the CLI
// calls it on every normal exit path.
func TestInstallInterruptCleanupStop(t *testing.T) {
	var buf bytes.Buffer
	stop := installInterruptCleanup(&buf)
	stop()
	if err := syscall.Kill(os.Getpid(), syscall.SIGURG); err != nil {
		t.Fatalf("kill: %v", err) // harmless signal; proves the process survives
	}
	if buf.Len() != 0 {
		t.Fatalf("cleanup wrote %q after stop", buf.String())
	}
}
