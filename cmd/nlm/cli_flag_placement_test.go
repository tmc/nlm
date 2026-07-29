package main

import (
	"bytes"
	"errors"
	"flag"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestGlobalFlagInventory(t *testing.T) {
	opts := globalOptions{}
	flags := newGlobalFlagSet(&opts)
	var got []string
	flags.VisitAll(func(flag *flag.Flag) {
		got = append(got, flag.Name)
	})
	slices.Sort(got)
	want := []string{
		"auth",
		"authuser",
		"cookies",
		"debug",
		"debug-dump-payload",
		"debug-field-mapping",
		"debug-parsing",
		"experimental",
		"version",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("global flags = %v, want %v", got, want)
	}
}

func TestTrueGlobalFlagsAcceptedAnywhere(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want globalOptions
	}{
		{
			name: "boolean before",
			args: []string{"--debug", "betool", "help"},
			want: globalOptions{debug: true},
		},
		{
			name: "boolean after",
			args: []string{"betool", "help", "--debug"},
			want: globalOptions{debug: true},
		},
		{
			name: "separated value before",
			args: []string{"--auth", "token", "betool", "help"},
			want: globalOptions{authToken: "token"},
		},
		{
			name: "attached value after",
			args: []string{"betool", "help", "--auth=token"},
			want: globalOptions{authToken: "token"},
		},
		{
			name: "authuser after",
			args: []string{"betool", "help", "--authuser", "3"},
			want: globalOptions{authUser: "3", authUserSet: true},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			inv, stderr, err := parseFlagPlacementInvocation(test.args...)
			if err != nil {
				t.Fatal(err)
			}
			if stderr != "" {
				t.Fatalf("stderr = %q, want empty", stderr)
			}
			if inv.globals.debug != test.want.debug ||
				inv.globals.authToken != test.want.authToken ||
				inv.globals.authUser != test.want.authUser ||
				inv.globals.authUserSet != test.want.authUserSet {
				t.Fatalf("globals = %+v, want selected fields %+v", inv.globals, test.want)
			}
		})
	}
}

func TestPreCommandFlagCompatibilityInventory(t *testing.T) {
	if preCommandFlagGraceRelease != "2026-07-29" {
		t.Fatalf("grace release = %q", preCommandFlagGraceRelease)
	}
	if preCommandFlagRemovalIssue == "" {
		t.Fatal("pre-command flag removal issue is empty")
	}

	names := make([]string, 0, len(preCommandFlagCompatibility))
	for name := range preCommandFlagCompatibility {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			owner, spec := stableFlagOwner(t, name)
			flagArgs := commandFlagTestArgs(name, spec)

			before := append(append([]string(nil), flagArgs...), strings.Fields(owner.name)...)
			inv, stderr, err := parseFlagPlacementInvocation(before...)
			if err != nil {
				t.Fatalf("pre-command owner: %v", err)
			}
			if inv.name != owner.name {
				t.Fatalf("command = %q, want %q", inv.name, owner.name)
			}
			if !slices.Equal(inv.args[:len(flagArgs)], flagArgs) {
				t.Fatalf("command args = %v, want prefix %v", inv.args, flagArgs)
			}
			display := commandFlagDisplay(flagArgs[0])
			wantWarning := "command flag " + display + " before " + `"` + owner.name + `"` + " is deprecated"
			if !strings.Contains(stderr, wantWarning) {
				t.Fatalf("stderr = %q, want %q", stderr, wantWarning)
			}

			after := append(strings.Fields(owner.name), flagArgs...)
			inv, stderr, err = parseFlagPlacementInvocation(after...)
			if err != nil {
				t.Fatalf("post-command owner: %v", err)
			}
			if stderr != "" {
				t.Fatalf("post-command stderr = %q, want empty", stderr)
			}
			if !slices.Equal(inv.args, flagArgs) {
				t.Fatalf("post-command args = %v, want %v", inv.args, flagArgs)
			}

			nonOwner := append(append([]string(nil), flagArgs...), "refresh")
			_, _, err = parseFlagPlacementInvocation(nonOwner...)
			if !errors.Is(err, errBadArgs) {
				t.Fatalf("non-owner error = %v, want errBadArgs", err)
			}
			if name == "f" {
				if want := "command flag -f before the command path is ambiguous"; !strings.Contains(err.Error(), want) {
					t.Fatalf("error = %q, want %q", err, want)
				}
			} else if want := "flag " + display + ` is not valid for "refresh"`; !strings.Contains(err.Error(), want) {
				t.Fatalf("error = %q, want %q", err, want)
			}
		})
	}
}

func TestPreCommandFlagNormalization(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantName   string
		wantArgs   []string
		wantWarn   string
		wantErr    string
		wantAction invocationAction
	}{
		{
			name:     "short separated value",
			args:     []string{"-f", "prompt.txt", "chat", "nb"},
			wantName: "chat",
			wantArgs: []string{"-f", "prompt.txt", "nb"},
			wantWarn: `command flag -f before "chat" is deprecated`,
		},
		{
			name:     "short attached value",
			args:     []string{"-f=prompt.txt", "chat", "nb"},
			wantName: "chat",
			wantArgs: []string{"-f=prompt.txt", "nb"},
			wantWarn: `command flag -f before "chat" is deprecated`,
		},
		{
			name:     "plural ordered warning",
			args:     []string{"--name", "Title", "--mime", "text/plain", "source", "add", "nb", "-"},
			wantName: "source add",
			wantArgs: []string{"--name", "Title", "--mime", "text/plain", "nb", "-"},
			wantWarn: `command flags --name, --mime before "source add" are deprecated`,
		},
		{
			name:     "repeated flag warned once",
			args:     []string{"--json", "--json", "betool", "help"},
			wantName: "betool",
			wantArgs: []string{"--json", "--json", "help"},
			wantWarn: `command flag --json before "betool" is deprecated`,
		},
		{
			name:     "negative numeric value",
			args:     []string{"--poll-ms", "-1", "research", "nb", "query"},
			wantName: "research",
			wantArgs: []string{"--poll-ms", "-1", "nb", "query"},
			wantWarn: `command flag --poll-ms before "research" is deprecated`,
		},
		{
			name:     "dash-prefixed value",
			args:     []string{"--prompt-file", "-prompt.txt", "chat", "nb"},
			wantName: "chat",
			wantArgs: []string{"--prompt-file", "-prompt.txt", "nb"},
			wantWarn: `command flag --prompt-file before "chat" is deprecated`,
		},
		{
			name:     "source read deprecated alias",
			args:     []string{"--markdown", "source", "read", "nb", "source"},
			wantName: "source read",
			wantArgs: []string{"--markdown", "nb", "source"},
			wantWarn: `command flag --markdown before "source read" is deprecated`,
		},
		{
			name:     "compatibility command",
			args:     []string{"--json", "sources", "nb"},
			wantName: "sources",
			wantArgs: []string{"--json", "nb"},
			wantWarn: `command flag --json before "sources" is deprecated`,
		},
		{
			name:     "separator after pre-command flag",
			args:     []string{"--json", "--", "betool", "help"},
			wantName: "betool",
			wantArgs: []string{"--json", "help"},
			wantWarn: `command flag --json before "betool" is deprecated`,
		},
		{
			name:     "literal dash operand",
			args:     []string{"source", "add", "nb", "-"},
			wantName: "source add",
			wantArgs: []string{"nb", "-"},
		},
		{
			name:     "command separator preserves dash filename",
			args:     []string{"source", "add", "nb", "--", "--name"},
			wantName: "source add",
			wantArgs: []string{"nb", "--", "--name"},
		},
		{
			name:       "leading separator blocks normalization",
			args:       []string{"--", "--json", "betool", "help"},
			wantErr:    "invalid arguments",
			wantAction: invocationRootHelp,
		},
		{
			name:    "f non-owner is ambiguous",
			args:    []string{"-f", "prompt.txt", "refresh"},
			wantErr: "command flag -f before the command path is ambiguous",
		},
		{
			name:    "known non-owner is invalid",
			args:    []string{"--json", "refresh"},
			wantErr: `flag --json is not valid for "refresh"`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			inv, stderr, err := parseFlagPlacementInvocation(test.args...)
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("error = %v, want containing %q", err, test.wantErr)
				}
				if test.wantAction != 0 && inv.action != test.wantAction {
					t.Fatalf("action = %v, want %v", inv.action, test.wantAction)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if inv.name != test.wantName || !reflect.DeepEqual(inv.args, test.wantArgs) {
				t.Fatalf("name, args = %q, %v; want %q, %v", inv.name, inv.args, test.wantName, test.wantArgs)
			}
			if test.wantWarn == "" {
				if stderr != "" {
					t.Fatalf("stderr = %q, want empty", stderr)
				}
			} else if !strings.Contains(stderr, test.wantWarn) {
				t.Fatalf("stderr = %q, want %q", stderr, test.wantWarn)
			}
		})
	}
}

func TestPreCommandFlagKeepsPostCommandLastWins(t *testing.T) {
	inv, _, err := parseFlagPlacementInvocation(
		"--name", "old",
		"source", "add",
		"--name", "new",
		"nb", "-",
	)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := parseBoundCommand(inv.cmd, inv.name, inv.args, inv.globals)
	if err != nil {
		t.Fatal(err)
	}
	if got := parsedStringFlag(parsed, "name", ""); got != "new" {
		t.Fatalf("name = %q, want new", got)
	}
}

func parseFlagPlacementInvocation(args ...string) (invocation, string, error) {
	var stderr bytes.Buffer
	inv, err := parseInvocation(args, func(string) string { return "" }, nil, &stderr)
	return inv, stderr.String(), err
}

func stableFlagOwner(t *testing.T, name string) (*command, flagSpec) {
	t.Helper()
	kind := preCommandFlagCompatibility[name]
	for i := range commands {
		command := &commands[i]
		if command.surface != surfaceStable || command.hidden {
			continue
		}
		for _, spec := range commandFlagsForSurface(command.spec, command.surfaceSpec) {
			if spec.Name != name && !containsString(spec.Aliases, name) {
				continue
			}
			specKind := preCommandBoolFlag
			if spec.Value != "" && !spec.OptionalValue {
				specKind = preCommandValueFlag
			}
			if specKind == kind {
				return command, spec
			}
		}
	}
	t.Fatalf("no stable owner for command flag %q", name)
	return nil, flagSpec{}
}

func commandFlagTestArgs(name string, spec flagSpec) []string {
	prefix := "--"
	if len(name) == 1 {
		prefix = "-"
	}
	arg := prefix + name
	if spec.OptionalValue {
		return []string{arg + "=1"}
	}
	if spec.Value != "" {
		return []string{arg, "value"}
	}
	return []string{arg}
}
