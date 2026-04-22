package api

import (
	"errors"
	"testing"

	"github.com/tmc/nlm/internal/batchexecute"
)

func TestWrapSourceAddError(t *testing.T) {
	t.Parallel()

	precondition := &batchexecute.APIError{
		ErrorCode: &batchexecute.ErrorCode{
			Code: 9,
		},
	}
	got := wrapSourceAddError("add source from URL", precondition)
	if !errors.Is(got, ErrSourceCapReached) {
		t.Fatalf("wrapSourceAddError() did not wrap ErrSourceCapReached: %v", got)
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
