package api

import (
	"net/http"
	"testing"
)

func TestNewAppliesClientConfiguration(t *testing.T) {
	client := New(
		Credentials{AuthToken: "auth-token", Cookies: "cookies"},
		WithAuthUser("2"),
		WithUseDirectRPC(true),
		WithSkipSources(true),
	)

	if got := client.rpc.Config.AuthToken; got != "auth-token" {
		t.Errorf("AuthToken = %q, want auth-token", got)
	}
	if got := client.rpc.Config.Cookies; got != "cookies" {
		t.Errorf("Cookies = %q, want cookies", got)
	}
	if got := client.rpc.Config.URLParams["authuser"]; got != "2" {
		t.Errorf("authuser URL parameter = %q, want 2", got)
	}
	if got := client.rpc.Config.Headers["x-goog-authuser"]; got != "2" {
		t.Errorf("x-goog-authuser header = %q, want 2", got)
	}
	if !client.config.UseDirectRPC {
		t.Error("UseDirectRPC = false, want true")
	}
	if !client.config.SkipSources {
		t.Error("SkipSources = false, want true")
	}
}

func TestWithDefaultAuthUserOmitsCarriers(t *testing.T) {
	for _, value := range []string{"", "0"} {
		t.Run("value="+value, func(t *testing.T) {
			client := New(Credentials{}, WithAuthUser(value))
			if _, ok := client.rpc.Config.URLParams["authuser"]; ok {
				t.Fatalf("authuser URL parameter present for %q", value)
			}
			if _, ok := client.rpc.Config.Headers["x-goog-authuser"]; ok {
				t.Fatalf("x-goog-authuser header present for %q", value)
			}
		})
	}
}

func TestDirectAuthUserCarriers(t *testing.T) {
	tests := []struct {
		name       string
		authUser   string
		wantURL    string
		wantHeader string
	}{
		{"default account", "", "https://notebook.google.com/upload/_/", ""},
		{"explicit account", "2", "https://notebook.google.com/upload/_/?authuser=2", "2"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			header := make(http.Header)
			setAuthUserHeader(header, test.authUser)
			if got := uploadURL(test.authUser); got != test.wantURL {
				t.Fatalf("uploadURL(%q) = %q, want %q", test.authUser, got, test.wantURL)
			}
			if got := header.Get("X-Goog-AuthUser"); got != test.wantHeader {
				t.Fatalf("X-Goog-AuthUser = %q, want %q", got, test.wantHeader)
			}
		})
	}
}
