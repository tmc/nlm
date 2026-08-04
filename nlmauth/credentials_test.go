package nlmauth

import (
	"errors"
	"testing"
)

func TestSaveLoadSessionRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	// Clear any inherited environment so LoadSession reflects the store only.
	for _, k := range []string{"NLM_AUTH_TOKEN", "NLM_COOKIES", "NLM_AUTHUSER"} {
		t.Setenv(k, "")
	}

	want := Session{
		Credentials: Credentials{
			AuthToken: "at-token",
			Cookies:   `SAPISID=abc; other="q"`,
			AuthUser:  "1",
		},
		BrowserProfile: "Default",
		SessionID:      "sess-123",
		BLParam:        "boq_20260101",
		SignalerAuth:   "signaler-xyz",
	}
	if err := Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := LoadSession()
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if got != want {
		t.Fatalf("LoadSession = %+v, want %+v", got, want)
	}
}

func TestLoadSessionMissingStore(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	got, err := LoadSession()
	if err != nil {
		t.Fatalf("LoadSession on missing store: %v", err)
	}
	if (got != Session{}) {
		t.Fatalf("LoadSession = %+v, want zero Session", got)
	}
}

func TestLoadEnvOverridesStore(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := Save(Session{Credentials: Credentials{AuthToken: "stored-at", Cookies: "stored-cookies", AuthUser: "0"}}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	t.Setenv("NLM_AUTH_TOKEN", "env-at")
	t.Setenv("NLM_COOKIES", "")
	t.Setenv("NLM_AUTHUSER", "")

	got, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.AuthToken != "env-at" {
		t.Errorf("AuthToken = %q, want env value %q", got.AuthToken, "env-at")
	}
	if got.Cookies != "stored-cookies" {
		t.Errorf("Cookies = %q, want stored value %q", got.Cookies, "stored-cookies")
	}
	if got.AuthUser != "0" {
		t.Errorf("AuthUser = %q, want stored value %q", got.AuthUser, "0")
	}
}

func TestLoadNoCredentials(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for _, k := range []string{"NLM_AUTH_TOKEN", "NLM_COOKIES", "NLM_AUTHUSER"} {
		t.Setenv(k, "")
	}
	if _, err := Load(); !errors.Is(err, ErrNoCredentials) {
		t.Fatalf("Load error = %v, want ErrNoCredentials", err)
	}
}
