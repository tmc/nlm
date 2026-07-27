package main

import (
	"os"
	"testing"
)

func TestParseAuthFlagsInterleaved(t *testing.T) {
	opts, remaining, err := parseAuthFlagsWithOptions([]string{
		"Work", "--debug", "--authuser", "2", "--cdp-url", "ws://localhost:9222",
	}, globalOptions{chromeProfile: "Default"})
	if err != nil {
		t.Fatalf("parseAuthFlagsWithOptions: %v", err)
	}
	if opts.ProfileName != "Default" {
		t.Fatalf("ProfileName = %q, want inherited Default", opts.ProfileName)
	}
	if !opts.Debug {
		t.Fatalf("Debug = false, want true")
	}
	if opts.AuthUser != "2" {
		t.Fatalf("AuthUser = %q, want 2", opts.AuthUser)
	}
	if opts.RemoteCDPURL != "ws://localhost:9222" {
		t.Fatalf("RemoteCDPURL = %q, want ws://localhost:9222", opts.RemoteCDPURL)
	}
	if len(remaining) != 1 || remaining[0] != "Work" {
		t.Fatalf("remaining = %v, want [Work]", remaining)
	}
}

func TestParseAuthFlagsDoesNotInheritAuthUser(t *testing.T) {
	t.Setenv("NLM_AUTHUSER", "1")

	opts, _, err := parseAuthFlagsWithOptions(nil, globalOptions{authUser: os.Getenv("NLM_AUTHUSER")})
	if err != nil {
		t.Fatalf("parseAuthFlagsWithOptions: %v", err)
	}
	if opts.AuthUser != "" {
		t.Fatalf("AuthUser = %q, want empty", opts.AuthUser)
	}
}

func TestParseAuthFlagsUsesExplicitGlobalAuthUser(t *testing.T) {
	opts, _, err := parseAuthFlagsWithOptions(nil, globalOptions{authUser: "2", authUserSet: true})
	if err != nil {
		t.Fatalf("parseAuthFlagsWithOptions: %v", err)
	}
	if opts.AuthUser != "2" {
		t.Fatalf("AuthUser = %q, want 2", opts.AuthUser)
	}
}
