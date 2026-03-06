package main

import "testing"

func TestParseAuthFlags_DefaultAuthUser(t *testing.T) {
	t.Setenv("NLM_AUTHUSER", "")

	opts, remaining, err := parseAuthFlags([]string{})
	if err != nil {
		t.Fatalf("parseAuthFlags returned error: %v", err)
	}
	if len(remaining) != 0 {
		t.Fatalf("expected no remaining args, got %v", remaining)
	}
	if opts.AuthUser != "0" {
		t.Fatalf("expected default AuthUser to be 0, got %q", opts.AuthUser)
	}
}

func TestParseAuthFlags_AuthUserFromEnv(t *testing.T) {
	t.Setenv("NLM_AUTHUSER", "1")

	opts, _, err := parseAuthFlags([]string{})
	if err != nil {
		t.Fatalf("parseAuthFlags returned error: %v", err)
	}
	if opts.AuthUser != "1" {
		t.Fatalf("expected AuthUser from env to be 1, got %q", opts.AuthUser)
	}
}

func TestParseAuthFlags_AuthUserFlagOverridesEnv(t *testing.T) {
	t.Setenv("NLM_AUTHUSER", "2")

	opts, _, err := parseAuthFlags([]string{"-authuser", "1"})
	if err != nil {
		t.Fatalf("parseAuthFlags returned error: %v", err)
	}
	if opts.AuthUser != "1" {
		t.Fatalf("expected AuthUser from flag to be 1, got %q", opts.AuthUser)
	}
}

func TestParseAuthFlags_AuthUserShortFlag(t *testing.T) {
	t.Setenv("NLM_AUTHUSER", "")

	opts, _, err := parseAuthFlags([]string{"-au", "3"})
	if err != nil {
		t.Fatalf("parseAuthFlags returned error: %v", err)
	}
	if opts.AuthUser != "3" {
		t.Fatalf("expected AuthUser from short flag to be 3, got %q", opts.AuthUser)
	}
}
