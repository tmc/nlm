package main

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestCommandSpecsCoverRegistry(t *testing.T) {
	if got, want := len(commandSpecs), 87; got != want {
		t.Fatalf("command specs = %d, want %d", got, want)
	}
	if got, want := len(groupedCommandSurfaces), 57; got != want {
		t.Fatalf("grouped surfaces = %d, want %d", got, want)
	}
	if got, want := len(commands), 144; got != want {
		t.Fatalf("bound commands = %d, want %d", got, want)
	}

	ids := make(map[commandID]bool)
	paths := make(map[string]bool)
	for _, spec := range commandSpecs {
		if ids[spec.ID] {
			t.Errorf("duplicate command ID %q", spec.ID)
		}
		ids[spec.ID] = true
		if spec.Decode == nil {
			t.Errorf("%s has no Decode", spec.ID)
		}
		if spec.parse == nil {
			t.Errorf("%s has no parser", spec.ID)
		}
		if len(spec.Forms) == 0 {
			t.Errorf("%s has no forms", spec.ID)
		}
		flagNames := make(map[string]bool)
		for _, flag := range spec.Flags {
			for _, name := range append([]string{flag.Name}, flag.Aliases...) {
				if name == "" {
					t.Errorf("%s has an empty flag name", spec.ID)
				}
				if flagNames[name] {
					t.Errorf("%s has duplicate flag %q", spec.ID, name)
				}
				flagNames[name] = true
			}
		}
		for i := range spec.Surfaces {
			surface := &spec.Surfaces[i]
			if surface.Surface == surfaceStable && len(surface.Forms) == 0 && len(spec.Forms) == 0 {
				t.Errorf("%s stable surface has no executable form", strings.Join(surface.Path, " "))
			}
			if surface.Surface == surfaceCompatibility && len(surface.Replacement) == 0 {
				t.Errorf("%s compatibility surface has no replacement", strings.Join(surface.Path, " "))
			}
			for _, path := range append([][]string{surface.Path}, surface.Aliases...) {
				name := strings.Join(path, " ")
				if paths[name] {
					t.Errorf("duplicate command path %q", name)
				}
				paths[name] = true
			}
		}
	}
	for i := range commands {
		if commands[i].spec == nil || commands[i].surfaceSpec == nil {
			t.Errorf("command %q is not spec-bound", commands[i].name)
		}
	}
}

func TestCommandSpecSynopses(t *testing.T) {
	for i := range commands {
		cmd := &commands[i]
		got := commandSynopsis(cmd.spec, cmd.surfaceSpec)
		want := cmd.argsUsage
		if inventory, ok := phase2InventorySynopses[cmd.spec.ID]; ok {
			want = inventory
		}
		if got != want {
			t.Errorf("%s synopsis = %q, want %q", cmd.name, got, want)
		}
	}
}

func TestCommandHelpUsesSpecSynopsis(t *testing.T) {
	for i := range commands {
		cmd := &commands[i]
		for _, path := range append([]string{cmd.name}, cmd.aliases...) {
			help := captureCommandStderr(t, func() {
				printCommandHelp(path, cmd)
			})
			first, _, _ := strings.Cut(help, "\n")
			title := cmd.surfaceSpec.Help.UsageTitle
			if title == "" {
				title = "usage"
			}
			want := fmt.Sprintf("%s: nlm %s", title, path)
			if synopsis := commandSynopsis(cmd.spec, cmd.surfaceSpec); synopsis != "" {
				want += " " + synopsis
			} else if cmd.surfaceSpec.Help.UsageTitle == "" {
				want += " "
			}
			if first != want {
				t.Errorf("%s detailed synopsis = %q, want %q", path, first, want)
			}
		}
	}
}

func TestCommandHelpCoversVisibleFlags(t *testing.T) {
	for _, spec := range commandSpecs {
		for i := range spec.Surfaces {
			surface := &spec.Surfaces[i]
			if surface.Help.UsageTitle == "" {
				continue
			}
			for _, flag := range spec.Flags {
				if flag.Visibility != flagVisible {
					continue
				}
				if !strings.Contains(surface.Help.Body, "-"+flag.Name) {
					t.Errorf("%s detailed help omits --%s", joinCommandPath(surface.Path), flag.Name)
				}
			}
		}
	}
}

var phase2InventorySynopses = map[commandID]string{
	"rm":                  "<notebook-id>",
	"add":                 "[flags] <notebook-id> <source...>",
	"sync":                "[flags] <notebook-id> [path...]",
	"sync-pack":           "[flags] [path...]",
	"label-attach":        "<notebook-id> <label-id|name> <source-id|name>",
	"app-create":          "[flags] <notebook-id> <instructions...>",
	"mindmap-create":      "[flags] <notebook-id> <instructions...>",
	"create-audio":        "[flags] <notebook-id> <instructions...>",
	"create-video":        "[flags] <notebook-id> <instructions...>",
	"create-slides":       "[flags] <notebook-id> [instructions...]",
	"deck-download":       "[flags] <notebook-id>",
	"download slide-deck": "[flags] <notebook-id>",
	"export-flashcards":   "[flags] <artifact-id>",
	"update-artifact":     "[--name <name>] <artifact-id> [title]",
	"source-guide":        "[flags] <notebook-id> [source-id...]",
	"generate-chat":       "[flags] <notebook-id> [prompt...]",
	"create-report":       "[flags] <notebook-id> <report-type> [description...]",
	"generate-report":     "[flags] <notebook-id>",
	"chat":                "[flags] <notebook-id> [conversation-id | prompt...]",
	"chat-show":           "[flags] <notebook-id> [conversation-id]",
	"chat-config": "<notebook-id> goal default | <notebook-id> goal custom <prompt...> | " +
		"<notebook-id> length <default|longer|shorter>",
	"research": "[flags] <notebook-id> <query...>",
	"auth":     "[login] [options] [profile-name]",
	"betool":   "<mode> [flags] [file...]",
	"refresh":  "",
}

func TestParseCommandFormCardinality(t *testing.T) {
	form := commandForm{Parts: []operandSpec{
		{Name: "notebook", Cardinality: cardinalityRequired},
		{Name: "source", Cardinality: cardinalityOneOrMore},
		{Name: "label", Cardinality: cardinalityOptional},
	}}
	tests := []struct {
		name string
		args []string
		want map[string][]string
		ok   bool
	}{
		{
			name: "required and repeated",
			args: []string{"nb", "a"},
			want: map[string][]string{"notebook": {"nb"}, "source": {"a"}},
			ok:   true,
		},
		{
			name: "backtracks for optional tail",
			args: []string{"nb", "a", "b", "label"},
			want: map[string][]string{
				"notebook": {"nb"},
				"source":   {"a", "b", "label"},
			},
			ok: true,
		},
		{name: "missing repeated", args: []string{"nb"}, ok: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := parseCommandForm(form, test.args)
			if ok != test.ok {
				t.Fatalf("ok = %v, want %v", ok, test.ok)
			}
			if !reflect.DeepEqual(got.Args, test.want) {
				t.Errorf("args = %#v, want %#v", got.Args, test.want)
			}
		})
	}
}

func TestParseCommandFormLiteral(t *testing.T) {
	form := commandForm{Parts: []operandSpec{
		{Literal: "set", Cardinality: cardinalityRequired},
		{Name: "key", Cardinality: cardinalityRequired},
		{Name: "value", Cardinality: cardinalityRequired},
	}}
	got, ok := parseCommandForm(form, []string{"set", "theme", "dark"})
	if !ok {
		t.Fatal("form did not match")
	}
	if want := []string{"theme"}; !reflect.DeepEqual(got.Args["key"], want) {
		t.Errorf("key = %q, want %q", got.Args["key"], want)
	}
	if _, ok := parseCommandForm(form, []string{"get", "theme", "dark"}); ok {
		t.Fatal("wrong literal matched")
	}
}

func TestParseCommandFlags(t *testing.T) {
	specs := []flagSpec{
		{Name: "format", Aliases: []string{"f"}, Value: "format"},
		{Name: "open"},
		{Name: "excerpt", Value: "n", OptionalValue: true},
	}
	flags, operands, err := parseCommandFlags(specs, []string{
		"--format=html", "--open=false", "--excerpt", "--excerpt=80",
		"notebook", "--unknown", "-f", "text", "--", "--literal",
	})
	if err != nil {
		t.Fatal(err)
	}
	wantFlags := map[string][]string{
		"format":  {"html", "text"},
		"open":    {"false"},
		"excerpt": {"true", "80"},
	}
	if !reflect.DeepEqual(flags, wantFlags) {
		t.Errorf("flags = %#v, want %#v", flags, wantFlags)
	}
	wantOperands := []string{"notebook", "--unknown", "--literal"}
	if !reflect.DeepEqual(operands, wantOperands) {
		t.Errorf("operands = %q, want %q", operands, wantOperands)
	}
}

func TestParseCommandSpecSurfaceForms(t *testing.T) {
	spec := commandSpec{
		Forms: []commandForm{{Parts: []operandSpec{
			{Name: "notebook", Cardinality: cardinalityRequired},
			{Name: "source", Cardinality: cardinalityRequired},
		}, Constraints: []constraint{
			constraintFunc(func(parsed parsedCommand) error {
				if len(parsed.Args["notebook"]) != 1 {
					return errBadArgs
				}
				return nil
			}),
		}}},
	}
	stable := commandSurfaceSpec{Path: []string{"source", "read"}}
	compat := commandSurfaceSpec{
		Path: []string{"read-source"},
		Forms: []commandForm{{Parts: []operandSpec{
			{Name: "source", Cardinality: cardinalityRequired},
			{Name: "notebook", Cardinality: cardinalityOptional},
		}}},
	}
	got, err := parseCommandSpec(&spec, &stable, []string{"nb", "src"}, globalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Args["notebook"][0] != "nb" || got.Args["source"][0] != "src" {
		t.Fatalf("stable args = %#v", got.Args)
	}
	got, err = parseCommandSpec(&spec, &compat, []string{"src", "nb"}, globalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Args["notebook"][0] != "nb" || got.Args["source"][0] != "src" {
		t.Fatalf("compat args = %#v", got.Args)
	}
}
