package api

import (
	"errors"
	"strings"
	"testing"

	"github.com/tmc/nlm/internal/batchexecute"
)

func TestWrapSourceAddError(t *testing.T) {
	t.Parallel()

	// Code-9 ("Failed precondition") arrives from the server with no
	// machine-readable discriminator — it can be cap-reached, oversize
	// payload, malformed envelope, or server policy. The wrapper must not
	// auto-classify; it should preserve the underlying APIError so the
	// caller (which may have out-of-band evidence) decides.
	precondition := &batchexecute.APIError{
		ErrorCode: &batchexecute.ErrorCode{
			Code: 9,
		},
	}
	got := wrapSourceAddError("add source from URL", precondition)
	if errors.Is(got, ErrSourceCapReached) {
		t.Fatalf("wrapSourceAddError() auto-wrapped code-9 as ErrSourceCapReached: %v", got)
	}
	var apiErr *batchexecute.APIError
	if !errors.As(got, &apiErr) {
		t.Fatalf("wrapSourceAddError() did not preserve APIError: %v", got)
	}

	other := &batchexecute.APIError{
		ErrorCode: &batchexecute.ErrorCode{
			Code: 3,
		},
	}
	got = wrapSourceAddError("add source from URL", other)
	if errors.Is(got, ErrSourceCapReached) {
		t.Fatalf("wrapSourceAddError() wrapped ErrSourceCapReached for non-precondition error: %v", got)
	}
}

// TestAddSourceFromTextTooLarge locks in the client-side size guard. Without
// it, 13MB+ payloads hit the server and come back as code-9 "failed
// precondition" which the wrapper mislabels as ErrSourceCapReached — the
// exact bug session 275D24EE reported on 2026-04-24.
func TestAddSourceFromTextTooLarge(t *testing.T) {
	t.Parallel()

	c := &Client{}
	oversize := strings.Repeat("a", MaxTextSourceBytes+1)
	_, err := c.AddSourceFromText("nb-1", oversize, "big")
	if err == nil {
		t.Fatalf("AddSourceFromText accepted oversize payload")
	}
	if !errors.Is(err, ErrSourceTooLarge) {
		t.Fatalf("err = %v, want ErrSourceTooLarge", err)
	}
	if errors.Is(err, ErrSourceCapReached) {
		t.Fatalf("err wrapped ErrSourceCapReached (the bug we fixed): %v", err)
	}
	// Message should include the actual byte counts so users can tell how
	// far over they are.
	if !strings.Contains(err.Error(), "big") {
		t.Fatalf("err = %q, want title in message", err)
	}
}
