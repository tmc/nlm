package main

import (
	"testing"
)

func TestParseAuthFlagsInterleaved(t *testing.T) {
	args := parseAuthCommandForTest(t, []string{
		"Work", "--debug", "--authuser", "2", "--cdp-url", "ws://localhost:9222",
	}, globalOptions{chromeProfile: "Default"})
	if args.Options.ProfileName != "Default" {
		t.Fatalf("ProfileName = %q, want inherited Default", args.Options.ProfileName)
	}
	if !args.Options.Debug {
		t.Fatalf("Debug = false, want true")
	}
	if args.Options.AuthUser != "2" {
		t.Fatalf("AuthUser = %q, want 2", args.Options.AuthUser)
	}
	if args.Options.RemoteCDPURL != "ws://localhost:9222" {
		t.Fatalf("RemoteCDPURL = %q, want ws://localhost:9222", args.Options.RemoteCDPURL)
	}
	if len(args.Remaining) != 1 || args.Remaining[0] != "Work" {
		t.Fatalf("remaining = %v, want [Work]", args.Remaining)
	}
}

// A re-login with no -authuser stays on the account the last login used. The
// environment carries it: loadStoredEnv publishes ~/.nlm/env before dispatch.
func TestParseAuthFlagsInheritsStoredAuthUser(t *testing.T) {
	t.Setenv("NLM_AUTHUSER", "1")

	args := parseAuthCommandForTest(t, nil, globalOptions{})
	if args.Options.AuthUser != "1" {
		t.Fatalf("AuthUser = %q, want 1", args.Options.AuthUser)
	}
}

// The default account is spelled as the absence of an index, never as 0, so a
// stored 0 must not become an explicit authuser=0 on the wire.
func TestParseAuthFlagsDoesNotInheritDefaultAuthUser(t *testing.T) {
	t.Setenv("NLM_AUTHUSER", "0")

	args := parseAuthCommandForTest(t, nil, globalOptions{})
	if args.Options.AuthUser != "" {
		t.Fatalf("AuthUser = %q, want empty", args.Options.AuthUser)
	}
}

// An explicit -authuser 0 selects the default account, clearing an inherited
// index rather than being ignored.
func TestParseAuthFlagsExplicitZeroClearsAuthUser(t *testing.T) {
	t.Setenv("NLM_AUTHUSER", "3")

	args := parseAuthCommandForTest(t, []string{"--authuser", "0"}, globalOptions{})
	if args.Options.AuthUser != "" {
		t.Fatalf("AuthUser = %q, want empty", args.Options.AuthUser)
	}
}

func TestParseAuthFlagsUsesExplicitGlobalAuthUser(t *testing.T) {
	args := parseAuthCommandForTest(t, nil, globalOptions{authUser: "2", authUserSet: true})
	if args.Options.AuthUser != "2" {
		t.Fatalf("AuthUser = %q, want 2", args.Options.AuthUser)
	}
}

func TestParseAuthFlagsDefersErrors(t *testing.T) {
	args := parseAuthCommandForTest(t, []string{"--keep-open", "bad"}, globalOptions{})
	if args.FlagError == nil {
		t.Fatal("FlagError = nil, want integer parse error")
	}

	args = parseAuthCommandForTest(t, []string{"--profile"}, globalOptions{})
	if args.FlagError == nil || args.FlagError.Error() != "flag needs an argument: --profile" {
		t.Fatalf("FlagError = %v", args.FlagError)
	}
}

func parseAuthCommandForTest(t *testing.T, values []string, globals globalOptions) authArgs {
	t.Helper()
	command, ok := lookupCommand("auth")
	if !ok {
		t.Fatal("auth command not found")
	}
	parsed, err := parseCommandSpec(command.spec, command.surfaceSpec, values, globals)
	if err != nil {
		t.Fatal(err)
	}
	return decodeAuthArgs(parsed)
}
