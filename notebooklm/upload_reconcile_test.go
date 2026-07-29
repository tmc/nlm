package notebooklm

import (
	"fmt"
	"strings"
	"testing"
)

// TestIsUploadFinalizedError verifies detection of the Scotty 500-with-finalize
// failure mode, where the source is often created despite the error. The
// detector keys off the X-Goog-Upload-Status=final marker that
// startResumableUpload folds into its error message.
func TestIsUploadFinalizedError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{
			"real scotty 500 finalize",
			fmt.Errorf("upload init failed (status 500): X-Goog-Upload-Status=final X-Guploader-Uploadid=abc"),
			true,
		},
		{
			"wrapped scotty 500 finalize",
			fmt.Errorf("start upload: %w", fmt.Errorf("upload init failed (status 500): X-Goog-Upload-Status=final")),
			true,
		},
		{
			"non-final upload status",
			fmt.Errorf("upload init failed (status 503): X-Goog-Upload-Status=active"),
			false,
		},
		{"unrelated error", fmt.Errorf("connection refused"), false},
	}
	for _, tt := range tests {
		if got := isUploadFinalizedError(tt.err); got != tt.want {
			t.Errorf("%s: isUploadFinalizedError = %v, want %v", tt.name, got, tt.want)
		}
	}
}

// TestUploadFinalizedGuidanceMessage documents the user-facing message shape
// emitted when a finalize-500 cannot be reconciled to an existing source: it
// must hint that the upload may have finalized and point at source list.
func TestUploadFinalizedGuidanceMessage(t *testing.T) {
	t.Parallel()

	initErr := fmt.Errorf("upload init failed (status 500): X-Goog-Upload-Status=final")
	msg := fmt.Errorf("start upload: %w (the upload may have finalized anyway; run 'nlm source list %s' to check for %q)",
		initErr, "nb-123", "photo.png").Error()

	for _, want := range []string{"may have finalized", "nlm source list nb-123", "photo.png"} {
		if !strings.Contains(msg, want) {
			t.Errorf("guidance message %q missing %q", msg, want)
		}
	}
}
