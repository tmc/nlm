package main

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/tmc/nlm/internal/auth"
	"golang.org/x/term"
)

// notebookLMURL is the target nlm authenticates against.
const notebookLMURL = "https://notebook.google.com"

// errAuthRequired reports that credentials are missing or expired and that a
// browser login was not attempted: there is no cached profile to re-harvest,
// or no user attached to watch the browser. It exits with the auth class.
var errAuthRequired = errors.New("session expired; run `nlm auth` in a terminal (or set NLM_AUTH_TOKEN and NLM_COOKIES)")

// authRequiredError is errAuthRequired carrying the server error that
// exposed the expiry. The message is the next step; the cause stays in the
// chain so callers can still classify the original failure.
type authRequiredError struct {
	cause error
}

func (e *authRequiredError) Error() string { return errAuthRequired.Error() }
func (e *authRequiredError) Unwrap() error { return e.cause }
func (e *authRequiredError) Is(target error) bool {
	return target == errAuthRequired
}

// authFailedError reports a browser or CDP login that was attempted and
// failed. Message names the cause in user terms and hint names the next thing
// to try; the library error text is kept in the chain for --debug.
type authFailedError struct {
	message string
	hint    string
	cause   error
}

func (e *authFailedError) Error() string { return "login failed: " + e.message }
func (e *authFailedError) Unwrap() error { return e.cause }
func (e *authFailedError) Hint() string  { return e.hint }

// profilesWithCookies lists the browser profiles that hold cookies for the
// target domain, most recently used first. It is a variable so tests do not
// depend on the browser profiles of the machine they run on.
var profilesWithCookies = auth.ProfilesWithCookies

// browserAuthAllowed reports whether nlm may open a browser here. It is a
// variable so tests can exercise the refresh path without a terminal.
var browserAuthAllowed = interactiveBrowserAuth

// interactiveBrowserAuth reports whether a browser login can be watched by a
// user: NLM_NONINTERACTIVE must be unset, stdin must be a terminal, and the
// platform must have a display server. A browser that nobody can see cannot
// pass a Google sign-in prompt, so nlm refuses to launch one.
func interactiveBrowserAuth() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("NLM_NONINTERACTIVE"))) {
	case "", "0", "false":
	default:
		return false
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return false
	}
	return hasDisplay()
}

// hasDisplay reports whether the platform can show a browser window. macOS
// and Windows always can; elsewhere an X11 or Wayland session must be present.
func hasDisplay() bool {
	switch runtime.GOOS {
	case "darwin", "windows":
		return true
	}
	return os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != ""
}

// loginFailure describes a failed login in user terms. profile is the profile
// that was tried and cdpURL the remote endpoint, if any; debug keeps the
// library error text in the printed message.
func loginFailure(cause error, profile, cdpURL, targetURL string, debug bool) *authFailedError {
	message := loginFailureCause(cause, profile, cdpURL)
	if debug {
		message += ": " + cause.Error()
	}
	hint := loginFailureHint(profile, cdpURL, targetURL)
	switch loginFailureCause(cause, profile, cdpURL) {
	case "no supported browser found (Chrome, Chrome Canary, or Brave)":
		hint = "install Brave or Chrome, then run `nlm auth` in a terminal"
	case "the browser did not become ready for sign-in":
		hint = "try: open Brave or Chrome and finish any startup dialogs, then run `nlm auth`; or use `nlm auth --cdp-url ws://localhost:9222`"
	}
	return &authFailedError{
		message: message,
		hint:    hint,
		cause:   cause,
	}
}

// loginFailureCause maps a browser or CDP error onto the handful of causes a
// user can act on. Unrecognised errors report the profile they came from;
// their text is still available under --debug.
func loginFailureCause(cause error, profile, cdpURL string) string {
	text := strings.ToLower(cause.Error())
	contains := func(subs ...string) bool {
		for _, sub := range subs {
			if strings.Contains(text, sub) {
				return true
			}
		}
		return false
	}
	switch {
	case cdpURL != "" && contains("connection refused", "no such host", "websocket", "dial tcp", "timeout"):
		return fmt.Sprintf("CDP endpoint %s is unreachable", cdpURL)
	case contains("websocket url timeout"):
		return "the browser did not become ready for sign-in"
	case contains("no valid browser profiles", "no profiles could authenticate", "no browser profile"):
		return "no browser profile has a signed-in notebook.google.com session"
	case contains("not logged in", "authentication page", "sign in", "signin"):
		return fmt.Sprintf("profile %s is not signed in to notebook.google.com", profile)
	case contains("insufficient cookies", "invalid token format", "no cookies", "auth data not found"):
		return fmt.Sprintf("profile %s has no notebook.google.com cookies", profile)
	case contains("deadline exceeded", "timeout", "timed out"):
		return fmt.Sprintf("timed out waiting for profile %s to authenticate", profile)
	case contains("executable file not found", "chrome not found", "could not find browser"):
		return "no supported browser found (Chrome, Chrome Canary, or Brave)"
	case contains("exit status", "signal:", "process exited"):
		return "the browser exited before authentication finished"
	default:
		return fmt.Sprintf("could not read credentials from profile %s", profile)
	}
}

// loginFailureHint names the next thing to try: another profile that already
// has cookies for the target, or attaching to a browser over CDP (R17).
func loginFailureHint(profile, cdpURL, targetURL string) string {
	const cdp = "nlm auth --cdp-url ws://localhost:9222"
	if profile == "nlm" && cdpURL == "" {
		return "try: nlm auth to sign in again, or " + cdp
	}
	if cdpURL != "" {
		return fmt.Sprintf("try: start the browser with --remote-debugging-port=9222, or nlm auth --profile %q", profile)
	}
	for _, name := range profilesWithCookies(targetURL) {
		if name != profile {
			return fmt.Sprintf("try: nlm auth --profile %q, or %s", name, cdp)
		}
	}
	return fmt.Sprintf("try: nlm auth --list-profiles to see what is available, or %s", cdp)
}

// errorHint returns the one-line next step an error carries, or "".
func errorHint(err error) string {
	var hinter interface{ Hint() string }
	if errors.As(err, &hinter) {
		return hinter.Hint()
	}
	return ""
}
