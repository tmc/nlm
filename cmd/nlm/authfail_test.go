package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/tmc/nlm/internal/batchexecute"
	"github.com/tmc/nlm/notebooklm"
)

// cachedProfile writes a stored env naming the browser profile the last login
// used, which is what makes a silent refresh possible.
func cachedProfile(t *testing.T, name string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("NLM_AUTO_REFRESH", "")
	t.Setenv("NLM_CDP_URL", "")
	if err := os.MkdirAll(filepath.Join(home, ".nlm"), 0700); err != nil {
		t.Fatal(err)
	}
	env := "NLM_BROWSER_PROFILE=\"" + name + "\"\n"
	if err := os.WriteFile(filepath.Join(home, ".nlm", "env"), []byte(env), 0600); err != nil {
		t.Fatal(err)
	}
}

// stubProfiles fixes the profile suggestion so the hint does not depend on
// the browser profiles of the machine running the test.
func stubProfiles(t *testing.T, names ...string) {
	t.Helper()
	old := profilesWithCookies
	profilesWithCookies = func(string) []string { return names }
	t.Cleanup(func() { profilesWithCookies = old })
}

// stubCredentials points the globals at fixed values and restores them.
func stubCredentials(t *testing.T, token, cookie string) {
	t.Helper()
	oldToken, oldCookies, oldDebug := authToken, cookies, debug
	t.Cleanup(func() { authToken, cookies, debug = oldToken, oldCookies, oldDebug })
	authToken, cookies, debug = token, cookie, false
}

// stubReharvest installs a fake browser login.
func stubReharvest(t *testing.T, f func(bool) (string, string, error)) {
	t.Helper()
	old := reharvestBrowserCredentials
	reharvestBrowserCredentials = f
	t.Cleanup(func() { reharvestBrowserCredentials = old })
}

// TestInteractiveBrowserAuthNeedsTerminal covers the guard from the outside:
// `go test` has no controlling terminal on stdin, so a browser must not be
// launched no matter what else is set.
func TestInteractiveBrowserAuthNeedsTerminal(t *testing.T) {
	t.Setenv("NLM_NONINTERACTIVE", "")
	t.Setenv("DISPLAY", ":0")
	if interactiveBrowserAuth() {
		t.Error("interactiveBrowserAuth() = true with a non-terminal stdin")
	}
	t.Setenv("NLM_NONINTERACTIVE", "1")
	if interactiveBrowserAuth() {
		t.Error("interactiveBrowserAuth() = true with NLM_NONINTERACTIVE=1")
	}
}

// TestRunRefusesBrowserWhenNonInteractive is the R12 transcript: one line,
// exit 3, and no browser.
func TestRunRefusesBrowserWhenNonInteractive(t *testing.T) {
	cachedProfile(t, "Work")
	stubCredentials(t, "stale-token", "stale-cookies")
	allowBrowserAuth(t, false)
	stubReharvest(t, func(bool) (string, string, error) {
		t.Error("non-interactive session launched a browser")
		return "", "", nil
	})

	attempts := 0
	cmd := testCommand("auth-noninteractive-test", func(*notebooklm.Client) error {
		attempts++
		return batchexecute.ErrUnauthorized
	})
	err := run(invocation{name: cmd.name, cmd: cmd})
	if !errors.Is(err, errAuthRequired) {
		t.Fatalf("run() error = %v, want errAuthRequired", err)
	}
	if !errors.Is(err, batchexecute.ErrUnauthorized) {
		t.Errorf("run() error lost the underlying 401: %v", err)
	}
	if attempts != 1 {
		t.Errorf("attempts = %d, want 1", attempts)
	}

	var buf bytes.Buffer
	code := reportRunError(&buf, err)
	want := "nlm: session expired; run `nlm auth` in a terminal (or set NLM_AUTH_TOKEN and NLM_COOKIES)\n" +
		"nlm: exit-class=auth (exit 3)\n"
	if buf.String() != want {
		t.Errorf("stderr =\n%q\nwant\n%q", buf.String(), want)
	}
	if code != exitAuth {
		t.Errorf("exit code = %d, want %d", code, exitAuth)
	}
}

// TestRunRefreshesOnlyOnce pins R13: a 401 that survives fresh credentials is
// reported, not refreshed again.
func TestRunRefreshesOnlyOnce(t *testing.T) {
	cachedProfile(t, "Work")
	stubCredentials(t, "old-token", "old-cookies")
	allowBrowserAuth(t, true)

	refreshes := 0
	stubReharvest(t, func(bool) (string, string, error) {
		refreshes++
		return "new-token", "new-cookies", nil
	})

	attempts := 0
	cmd := testCommand("auth-once-test", func(*notebooklm.Client) error {
		attempts++
		return batchexecute.ErrUnauthorized
	})
	err := run(invocation{name: cmd.name, cmd: cmd})
	if !errors.Is(err, batchexecute.ErrUnauthorized) {
		t.Fatalf("run() error = %v, want ErrUnauthorized", err)
	}
	if attempts != 2 || refreshes != 1 {
		t.Fatalf("attempts, refreshes = %d, %d; want 2, 1", attempts, refreshes)
	}
}

// TestRunReportsLoginFailure is the R15 transcript: one cause line in user
// terms, one next-step line, and exit 9.
func TestRunReportsLoginFailure(t *testing.T) {
	cachedProfile(t, "Default")
	stubCredentials(t, "stale-token", "stale-cookies")
	allowBrowserAuth(t, true)
	stubProfiles(t, "Default", "Profile 3")
	stubReharvest(t, func(bool) (string, string, error) {
		return "", "", errors.New("browser auth failed: insufficient cookies data")
	})

	cmd := testCommand("auth-failure-test", func(*notebooklm.Client) error {
		return batchexecute.ErrUnauthorized
	})

	stderr := captureStderr(t, func(func() string) {
		err := run(invocation{name: cmd.name, cmd: cmd})
		var buf bytes.Buffer
		if code := reportRunError(&buf, err); code != exitAuthFailed {
			t.Errorf("exit code = %d, want %d", code, exitAuthFailed)
		}
		want := "nlm: login failed: profile Default has no notebook.google.com cookies\n" +
			"nlm: try: nlm auth --profile \"Profile 3\", or nlm auth --cdp-url ws://localhost:9222\n" +
			"nlm: exit-class=auth-failed (exit 9)\n"
		if buf.String() != want {
			t.Errorf("report =\n%q\nwant\n%q", buf.String(), want)
		}
	})
	want := "nlm: session expired, re-authenticating via browser (profile Default)...\n"
	if stderr != want {
		t.Errorf("stderr = %q, want exactly %q", stderr, want)
	}
}

func TestLoginFailureCause(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		cdpURL string
		want   string
	}{
		{
			name: "no cookies",
			err:  errors.New("browser auth failed: insufficient cookies data"),
			want: "profile Default has no notebook.google.com cookies",
		},
		{
			name: "not signed in",
			err:  errors.New("redirected to authentication page - not logged in"),
			want: "profile Default is not signed in to notebook.google.com",
		},
		{
			name:   "cdp unreachable",
			err:    errors.New("dial tcp 127.0.0.1:9222: connection refused"),
			cdpURL: "ws://localhost:9222",
			want:   "CDP endpoint ws://localhost:9222 is unreachable",
		},
		{
			name: "timeout",
			err:  errors.New("context deadline exceeded"),
			want: "timed out waiting for profile Default to authenticate",
		},
		{
			name: "no profiles",
			err:  errors.New("no valid browser profiles found"),
			want: "no browser profile has a signed-in notebook.google.com session",
		},
		{
			name: "unrecognised",
			err:  errors.New("something the library did not explain"),
			want: "could not read credentials from profile Default",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := loginFailureCause(tt.err, "Default", tt.cdpURL); got != tt.want {
				t.Errorf("loginFailureCause() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestLoginFailureKeepsLibraryTextForDebug covers R15: the library text is
// hidden by default and available under --debug.
func TestLoginFailureKeepsLibraryTextForDebug(t *testing.T) {
	stubProfiles(t)
	cause := errors.New("chromedp: insufficient cookies data")

	quiet := loginFailure(cause, "Default", "", notebookLMURL, false)
	if got := quiet.Error(); got != "login failed: profile Default has no notebook.google.com cookies" {
		t.Errorf("quiet message = %q", got)
	}
	if !errors.Is(quiet, cause) {
		t.Error("quiet failure dropped the cause from the error chain")
	}

	loud := loginFailure(cause, "Default", "", notebookLMURL, true)
	if want := "login failed: profile Default has no notebook.google.com cookies: chromedp: insufficient cookies data"; loud.Error() != want {
		t.Errorf("debug message = %q, want %q", loud.Error(), want)
	}
}

// TestLoginFailureHintFallsBackWithoutAlternative keeps the hint honest when
// no other profile has cookies for the target.
func TestLoginFailureHintFallsBackWithoutAlternative(t *testing.T) {
	stubProfiles(t, "Default")
	want := "try: nlm auth --list-profiles to see what is available, or nlm auth --cdp-url ws://localhost:9222"
	if got := loginFailureHint("Default", "", notebookLMURL); got != want {
		t.Errorf("loginFailureHint() = %q, want %q", got, want)
	}
}
