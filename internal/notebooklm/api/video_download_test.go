package api

import (
	"strings"
	"testing"
)

func TestManualVideoDownloadError(t *testing.T) {
	t.Parallel()

	err := manualVideoDownloadError("notebook-123")
	want := "download manually from https://notebooklm.google.com/notebook/notebook-123"
	if err == nil || err.Error() == "" {
		t.Fatal("manualVideoDownloadError() returned nil/empty error")
	}
	if got := err.Error(); got == "" || !strings.Contains(got, want) {
		t.Fatalf("manualVideoDownloadError() = %q, want contains %q", got, want)
	}
}
