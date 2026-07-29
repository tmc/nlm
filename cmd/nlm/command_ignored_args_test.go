package main

import (
	"errors"
	"strings"
	"testing"
)

func TestCommandIgnoredArguments(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		args    []string
		wantErr string
	}{
		{name: "auth empty", path: "auth"},
		{name: "auth profile", path: "auth", args: []string{"Work"}},
		{name: "auth login", path: "auth", args: []string{"login"}},
		{name: "auth login profile", path: "auth", args: []string{"login", "Work"}},
		{name: "auth extra", path: "auth", args: []string{"Work", "extra"}, wantErr: `unexpected argument "extra" for "auth"`},
		{name: "auth login extra", path: "auth", args: []string{"login", "Work", "extra"}, wantErr: `unexpected argument "extra" for "auth"`},
		{name: "refresh empty", path: "refresh"},
		{name: "refresh extra", path: "refresh", args: []string{"extra"}, wantErr: `unexpected argument "extra" for "refresh"`},
		{name: "chat goal default", path: "chat config", args: []string{"nb", "goal", "default"}},
		{name: "chat goal custom", path: "chat config", args: []string{"nb", "goal", "custom", "answer", "briefly"}},
		{name: "chat length", path: "chat config", args: []string{"nb", "length", "longer"}},
		{name: "chat setting", path: "chat config", args: []string{"nb", "bogus"}, wantErr: `unexpected argument "bogus" for "chat config"`},
		{name: "chat goal mode", path: "chat config", args: []string{"nb", "goal", "bogus"}, wantErr: `unexpected argument "bogus" for "chat config"`},
		{name: "chat goal default extra", path: "chat config", args: []string{"nb", "goal", "default", "extra"}, wantErr: `unexpected argument "extra" for "chat config"`},
		{name: "chat length mode", path: "chat config", args: []string{"nb", "length", "huge"}, wantErr: `unexpected argument "huge" for "chat config"`},
		{name: "chat length extra", path: "chat config", args: []string{"nb", "length", "default", "extra"}, wantErr: `unexpected argument "extra" for "chat config"`},
		{name: "compat chat setting", path: "chat-config", args: []string{"nb", "bogus"}, wantErr: `unexpected argument "bogus" for "chat-config"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			command, ok := lookupCommand(tt.path)
			if !ok {
				t.Fatalf("lookupCommand(%q) failed", tt.path)
			}
			err := validateCommandArgs(command, tt.path, tt.args, globalOptions{})
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("validateCommandArgs() error = %v", err)
				}
				return
			}
			if err == nil || err.Error() != tt.wantErr {
				t.Fatalf("validateCommandArgs() error = %v, want %q", err, tt.wantErr)
			}
			if !errors.Is(err, errBadArgs) {
				t.Fatalf("errors.Is(%v, errBadArgs) = false", err)
			}
		})
	}
}

func TestIgnoredArgumentCLIError(t *testing.T) {
	var stderr strings.Builder
	code := runCLI(
		[]string{"refresh", "extra"},
		func(string) string { return "" },
		nil,
		&stderr,
	)
	if code != exitBadArgs {
		t.Fatalf("exit = %d, want %d", code, exitBadArgs)
	}
	if want := `nlm: unexpected argument "extra" for "refresh"`; !strings.Contains(stderr.String(), want) {
		t.Fatalf("stderr = %q, want %q", stderr.String(), want)
	}
}
