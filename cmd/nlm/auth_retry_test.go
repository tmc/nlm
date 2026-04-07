package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsAuthenticationError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "session invalid",
			err:  errors.New("remote session invalid"),
			want: true,
		},
		{
			name: "token expired",
			err:  errors.New("token expired while refreshing"),
			want: true,
		},
		{
			name: "login required",
			err:  errors.New("login required to continue"),
			want: true,
		},
		{
			name: "service unavailable",
			err:  errors.New("api error 3 (Unavailable): Service unavailable"),
			want: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := isAuthenticationError(tt.err); got != tt.want {
				t.Fatalf("isAuthenticationError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestPersistAuthToDiskPreservesSessionState(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("NLM_COOKIES", "")
	t.Setenv("NLM_AUTH_TOKEN", "")
	t.Setenv("NLM_BROWSER_PROFILE", "")
	t.Setenv("NLM_SESSION_ID", "")
	t.Setenv("NLM_BL_PARAM", "")

	if _, _, err := persistAuthToDisk("cookie-a", "token-a", "Default", "session-a", "bl-a"); err != nil {
		t.Fatalf("persistAuthToDisk() initial error = %v", err)
	}
	if _, _, err := persistAuthToDisk("cookie-b", "token-b", "", "", ""); err != nil {
		t.Fatalf("persistAuthToDisk() update error = %v", err)
	}

	if got := os.Getenv("NLM_SESSION_ID"); got != "session-a" {
		t.Fatalf("NLM_SESSION_ID = %q, want session-a", got)
	}
	if got := os.Getenv("NLM_BL_PARAM"); got != "bl-a" {
		t.Fatalf("NLM_BL_PARAM = %q, want bl-a", got)
	}

	data, err := os.ReadFile(filepath.Join(home, ".nlm", "env"))
	if err != nil {
		t.Fatalf("read env file: %v", err)
	}
	text := string(data)
	for _, want := range []string{
		`NLM_COOKIES="cookie-b"`,
		`NLM_AUTH_TOKEN="token-b"`,
		`NLM_BROWSER_PROFILE="Default"`,
		`NLM_SESSION_ID="session-a"`,
		`NLM_BL_PARAM="bl-a"`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("env file missing %q\n%s", want, text)
		}
	}
}
