package main

import (
	"errors"
	"strings"
	"testing"
)

func TestCommandFlagRejection(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		args    []string
		wantErr string
	}{
		{
			name: "owned flag",
			path: "notebook list",
			args: []string{"--json"},
		},
		{
			name:    "unknown flag",
			path:    "notebook list",
			args:    []string{"--bogus"},
			wantErr: `unknown flag --bogus for "notebook list"`,
		},
		{
			name:    "unknown attached flag",
			path:    "notebook list",
			args:    []string{"--bogus=value"},
			wantErr: `unknown flag --bogus for "notebook list"`,
		},
		{
			name:    "known flag on non-owner",
			path:    "notebook delete",
			args:    []string{"--json", "notebook"},
			wantErr: `flag --json is not valid for "notebook delete"`,
		},
		{
			name:    "global spelling in direct command grammar",
			path:    "notebook list",
			args:    []string{"--debug"},
			wantErr: `flag --debug is not valid for "notebook list"`,
		},
		{
			name: "unknown after separator is data",
			path: "source add",
			args: []string{"notebook", "--", "--bogus"},
		},
		{
			name: "known flag spelling as flag value",
			path: "source add",
			args: []string{"--name", "--debug", "notebook", "file"},
		},
		{
			name:    "negative operand requires separator",
			path:    "research",
			args:    []string{"notebook", "-1"},
			wantErr: `unknown flag -1 for "research"`,
		},
		{
			name: "negative operand after separator",
			path: "research",
			args: []string{"notebook", "--", "-1"},
		},
		{
			name: "single dash remains data",
			path: "source add",
			args: []string{"notebook", "-"},
		},
		{
			name:    "compatibility path in diagnostic",
			path:    "sources",
			args:    []string{"--bogus", "notebook"},
			wantErr: `unknown flag --bogus for "sources"`,
		},
		{
			name:    "compatibility remaining operand",
			path:    "rename-artifact",
			args:    []string{"artifact", "--bogus"},
			wantErr: `unknown flag --bogus for "rename-artifact"`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			command, ok := lookupCommand(test.path)
			if !ok {
				t.Fatalf("missing command %q", test.path)
			}
			err := validateCommandArgs(command, test.path, test.args, globalOptions{})
			if test.wantErr == "" {
				if err != nil {
					t.Fatalf("validate: %v", err)
				}
				return
			}
			if !errors.Is(err, errBadArgs) {
				t.Fatalf("error = %v, want errBadArgs", err)
			}
			if err.Error() != test.wantErr {
				t.Fatalf("error = %q, want %q", err, test.wantErr)
			}
		})
	}
}

func TestUnknownFlagCLIError(t *testing.T) {
	var stderr strings.Builder
	code := runCLI(
		[]string{"notebook", "list", "--bogus"},
		func(string) string { return "" },
		nil,
		&stderr,
	)
	if code != exitBadArgs {
		t.Fatalf("exit = %d, want %d", code, exitBadArgs)
	}
	if want := `nlm: unknown flag --bogus for "notebook list"`; !strings.Contains(stderr.String(), want) {
		t.Fatalf("stderr = %q, want %q", stderr.String(), want)
	}
}
