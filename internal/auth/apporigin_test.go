package auth

import "testing"

// TestDefaultTargetURLUsesAppOrigin pins the browser-auth default target to the
// NotebookLM app origin. The origin constant itself is covered by
// nlmauth.TestAppOrigin.
func TestDefaultTargetURLUsesAppOrigin(t *testing.T) {
	if got := defaultBrowserAuthOptions().TargetURL; got != appOrigin {
		t.Fatalf("default TargetURL = %q, want appOrigin %q", got, appOrigin)
	}
}
