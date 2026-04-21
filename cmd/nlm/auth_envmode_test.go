package main

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tmc/nlm/internal/batchexecute"
)

// TestShellQuote verifies the POSIX quoting used by auth --print-env.
func TestShellQuote(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in, want string
	}{
		{"", "''"},
		{"plain", "'plain'"},
		{"a=b; c=d", "'a=b; c=d'"},
		{"has'quote", `'has'\''quote'`},
		{"many''quotes", `'many'\'''\''quotes'`},
		{"spaces and $dollar", "'spaces and $dollar'"},
	}
	for _, tt := range tests {
		if got := shellQuote(tt.in); got != tt.want {
			t.Errorf("shellQuote(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// TestPrintAuthEnvFromEnvOnly verifies the env-var-only happy path:
// NLM_AUTH_TOKEN / NLM_COOKIES set, no ~/.nlm/env on disk, output is
// exactly two `export` lines and nothing else on stdout.
func TestPrintAuthEnvFromEnvOnly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("NLM_AUTH_TOKEN", "tok-123")
	t.Setenv("NLM_COOKIES", "SID=abc; HSID=xyz")
	t.Setenv("NLM_AUTHUSER", "1")

	var buf bytes.Buffer
	_, _, err := printAuthEnv(&buf)
	if err != nil {
		t.Fatalf("printAuthEnv() error = %v", err)
	}
	got := buf.String()
	want := "export NLM_AUTH_TOKEN='tok-123'\nexport NLM_COOKIES='SID=abc; HSID=xyz'\nexport NLM_AUTHUSER='1'\n"
	if got != want {
		t.Fatalf("printAuthEnv() output mismatch\n got: %q\nwant: %q", got, want)
	}
}

// TestPrintAuthEnvMissing verifies the no-credentials failure path.
func TestPrintAuthEnvMissing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("NLM_AUTH_TOKEN", "")
	t.Setenv("NLM_COOKIES", "")

	var buf bytes.Buffer
	_, _, err := printAuthEnv(&buf)
	if err == nil {
		t.Fatal("printAuthEnv() expected error, got nil")
	}
	if !strings.Contains(err.Error(), "no authenticated session found") {
		t.Fatalf("error message missing hint: %v", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("printAuthEnv() wrote %q to w on error; want empty", buf.String())
	}
}

// TestHasCachedProfile verifies the profile-presence helper that gates
// browser-auth retry on 401.
func TestHasCachedProfile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if hasCachedProfile() {
		t.Fatal("hasCachedProfile() = true on fresh HOME; want false")
	}
	if err := os.MkdirAll(filepath.Join(home, ".nlm"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".nlm", "env"), []byte(""), 0600); err != nil {
		t.Fatal(err)
	}
	if !hasCachedProfile() {
		t.Fatal("hasCachedProfile() = false after writing ~/.nlm/env; want true")
	}
}

// TestUnauthorizedFromBackendIsAuthError constructs a manual 401 response
// from a local httptest server (live captures cannot reliably reproduce a
// fresh 401) and verifies that the batchexecute stack surfaces it as an
// authentication error. This is the signal Phase 2 relies on to print
// the "session expired" message.
func TestUnauthorizedFromBackendIsAuthError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		io.WriteString(w, "Unauthorized")
	}))
	defer server.Close()

	host := strings.TrimPrefix(server.URL, "http://")
	cfg := batchexecute.Config{
		Host:       host,
		App:        "LabsTailwindUi",
		Cookies:    "SID=bad",
		AuthToken:  "bad-token",
		UseHTTP:    true,
		MaxRetries: 0,
	}
	client := batchexecute.NewClient(cfg)
	_, err := client.Execute([]batchexecute.RPC{{ID: "wXbhsf", Args: []interface{}{}}})
	if err == nil {
		t.Fatal("expected error from 401, got nil")
	}
	if !errors.Is(err, batchexecute.ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}
	if !isAuthenticationError(err) {
		t.Fatalf("isAuthenticationError(%v) = false; want true so the CLI prints the session-expired message", err)
	}
}
