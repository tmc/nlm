package notebooklm

import (
	"errors"
	"testing"

	"github.com/tmc/nlm/internal/batchexecute"
)

func TestClassifyCreateProjectError(t *testing.T) {
	t.Parallel()

	invalidArg := &batchexecute.APIError{
		ErrorCode: &batchexecute.ErrorCode{Code: 3, Type: batchexecute.ErrorTypeInvalidInput, Message: "Invalid argument"},
	}
	failedPrecondition := &batchexecute.APIError{
		ErrorCode: &batchexecute.ErrorCode{Code: 9, Type: batchexecute.ErrorTypeInvalidInput, Message: "Failed precondition"},
	}
	transient := &batchexecute.APIError{
		ErrorCode: &batchexecute.ErrorCode{Code: 13, Type: batchexecute.ErrorTypeServerError, Message: "Internal"},
	}
	plain := errors.New("network blew up")

	tests := []struct {
		name      string
		err       error
		count     int
		limit     int
		wantCap   bool
		wantOther error // expect errors.Is on this even when wantCap is false
	}{
		{
			name:    "nil error returns nil",
			err:     nil,
			count:   500,
			limit:   500,
			wantCap: false,
		},
		{
			name:      "strict cap: count == limit",
			err:       invalidArg,
			count:     500,
			limit:     500,
			wantCap:   true,
			wantOther: invalidArg,
		},
		{
			name:      "strict cap: count > limit",
			err:       invalidArg,
			count:     501,
			limit:     500,
			wantCap:   true,
			wantOther: invalidArg,
		},
		{
			// The exact bug-report scenario: nlm account showed 492/500,
			// CreateProject returned bare code-3 "Invalid argument", and the
			// strict >= check missed it. ListRecentlyViewedProjects
			// underreports (omits archived/shared) so the server can be at
			// the cap even when the client-visible count is below it. The
			// heuristic (within tolerance + a quota-shaped wire error)
			// catches this without forcing the user to also delete a
			// notebook just to learn that's the fix.
			name:      "near_cap_underreport_492_of_500_code_3_reclassifies",
			err:       invalidArg,
			count:     492,
			limit:     500,
			wantCap:   true,
			wantOther: invalidArg,
		},
		{
			name:      "near-cap heuristic: code 9 also triggers",
			err:       failedPrecondition,
			count:     485,
			limit:     500,
			wantCap:   true,
			wantOther: failedPrecondition,
		},
		{
			// Outside the tolerance band — too far below the limit to blame
			// quota even with a code-3. Genuine malformed input must still
			// surface as a non-precondition error so the caller can fix it.
			name:      "far below cap: code 3 not reclassified",
			err:       invalidArg,
			count:     400,
			limit:     500,
			wantCap:   false,
			wantOther: invalidArg,
		},
		{
			// In the tolerance band but the wire error is a 5xx-class server
			// failure, which is not a quota signal. Reclassifying would hide
			// the transient class and prevent the caller from retrying.
			name:      "near cap but transient: not reclassified",
			err:       transient,
			count:     495,
			limit:     500,
			wantCap:   false,
			wantOther: transient,
		},
		{
			name:    "plain error never reclassified (no APIError)",
			err:     plain,
			count:   495,
			limit:   500,
			wantCap: false,
		},
		{
			// Account-status fetch failed: limit unknown. Must not guess.
			name:      "missing limit leaves error unwrapped",
			err:       invalidArg,
			count:     -1,
			limit:     -1,
			wantCap:   false,
			wantOther: invalidArg,
		},
		{
			// limit known but count fetch failed (rare): also leave alone
			// rather than risk a false positive.
			name:      "missing count leaves error unwrapped",
			err:       invalidArg,
			count:     -1,
			limit:     500,
			wantCap:   false,
			wantOther: invalidArg,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyCreateProjectError(tt.err, tt.count, tt.limit)
			if tt.err == nil {
				if got != nil {
					t.Fatalf("classifyCreateProjectError(nil) = %v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatalf("classifyCreateProjectError(%v) = nil, want non-nil", tt.err)
			}
			if errors.Is(got, ErrNotebookCapReached) != tt.wantCap {
				t.Errorf("errors.Is(got, ErrNotebookCapReached) = %v, want %v; got = %v",
					!tt.wantCap, tt.wantCap, got)
			}
			if tt.wantOther != nil && !errors.Is(got, tt.wantOther) {
				t.Errorf("errors.Is(got, original) = false, want true; got = %v", got)
			}
			// When classified as cap-reached, the wrapping must carry the
			// observed count/limit out via NotebookCapError so the user-
			// facing rewriter can surface "492/500".
			if tt.wantCap {
				var capErr *NotebookCapError
				if !errors.As(got, &capErr) {
					t.Fatalf("errors.As(got, *NotebookCapError) = false; got = %v", got)
				}
				if capErr.Count != tt.count || capErr.Limit != tt.limit {
					t.Errorf("NotebookCapError = {count:%d limit:%d}, want {count:%d limit:%d}",
						capErr.Count, capErr.Limit, tt.count, tt.limit)
				}
			}
		})
	}
}
