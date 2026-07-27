package api

import "testing"

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
