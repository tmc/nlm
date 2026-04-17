package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
)

// TestNewListWriter_NonTTY verifies that when w is not a TTY the returned
// writer emits raw tab-separated output (no padding, no ANSI). Pipes get
// literal \t bytes so cut/awk/paste work.
func TestNewListWriter_NonTTY(t *testing.T) {
	// os.Pipe() returns non-TTY file descriptors by construction.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	defer r.Close()

	writer, flush := newListWriter(w)

	if _, err := fmt.Fprintln(writer, "ID\tTITLE\tN"); err != nil {
		t.Fatalf("write header: %v", err)
	}
	if _, err := fmt.Fprintln(writer, "short\tLong Title\t42"); err != nil {
		t.Fatalf("write row: %v", err)
	}
	if err := flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}
	_ = w.Close()

	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	got := string(out)

	// Non-TTY: tabs remain literal, no column padding.
	wantLines := []string{"ID\tTITLE\tN", "short\tLong Title\t42"}
	for _, want := range wantLines {
		if !strings.Contains(got, want+"\n") {
			t.Errorf("missing raw-TSV line %q in output %q", want, got)
		}
	}
	// Padded output would space-align the header "ID" past "short"; a
	// tabwriter against a 2-char-vs-5-char column would emit "ID   \t".
	// Raw TSV must not have multiple spaces after ID.
	if strings.Contains(got, "ID   ") || strings.Contains(got, "ID  ") {
		t.Errorf("non-TTY output has column padding; want literal tabs. got: %q", got)
	}
}
