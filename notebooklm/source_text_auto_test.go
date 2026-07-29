package notebooklm

import (
	"fmt"
	"testing"

	"github.com/tmc/nlm/internal/batchexecute"
)

func apiErrCode(code int) error {
	ec, _ := batchexecute.GetErrorCode(code)
	return &batchexecute.APIError{ErrorCode: ec}
}

// TestShouldDescendOnAutoChunkError locks in that auto-chunk only re-splits on
// retryable, payload-shaped server errors. A non-retryable state/policy
// rejection (notably code 9 "Failed precondition") must NOT descend — that was
// the cause of a tiny source re-submitting identical bytes down the whole
// schedule before "failed at schedule floor".
func TestShouldDescendOnAutoChunkError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"cap reached", ErrSourceCapReached, false},
		{"too large", ErrSourceTooLarge, false},
		{"code 9 failed precondition (non-retryable)", apiErrCode(9), false},
		{"code 7 permission denied", apiErrCode(7), false},
		{"code 16 unauthenticated", apiErrCode(16), false},
		{"code 13 internal (retryable, payload-shaped)", apiErrCode(13), true},
		{"code 14 unavailable (retryable)", apiErrCode(14), true},
		{"plain error descends as a last resort", fmt.Errorf("mystery"), true},
	}
	for _, tt := range tests {
		if got := shouldDescendOnAutoChunkError(tt.err); got != tt.want {
			t.Errorf("%s: shouldDescendOnAutoChunkError = %v, want %v", tt.name, got, tt.want)
		}
	}
}

// TestAutoChunkScheduleNextSkipsNoOpSplits verifies the descent guard: from a
// given level, the next level used must actually be smaller than the part, so
// re-splitting never re-submits the identical bytes. This mirrors the loop in
// uploadOne.
func TestAutoChunkScheduleNextSkipsNoOpSplits(t *testing.T) {
	t.Parallel()

	// A 1050-byte part: every schedule level is >= 1050 except the 4 KiB floor
	// is also > 1050, so there is no level that would split it — the guard must
	// run off the end of the schedule (terminal) rather than retry 13 times.
	part := 1050
	next := 1 // pretend we just failed at level 0
	for next < len(AutoChunkSchedule) && part <= AutoChunkSchedule[next] {
		next++
	}
	if next < len(AutoChunkSchedule) {
		t.Fatalf("a %d-byte part should exhaust the schedule, but stopped at level %d (size %d)",
			part, next, AutoChunkSchedule[next])
	}

	// A part larger than the floor but smaller than mid-levels must land on the
	// first level that is actually smaller than it.
	part = 100 * 1024 // 100 KiB
	next = 0
	for next < len(AutoChunkSchedule) && part <= AutoChunkSchedule[next] {
		next++
	}
	if next >= len(AutoChunkSchedule) || AutoChunkSchedule[next] >= part {
		t.Fatalf("a %d-byte part should descend to a smaller level, got idx %d", part, next)
	}
}
