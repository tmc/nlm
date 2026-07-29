package main

import (
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
		if len(spec.Forms) == 0 {
			t.Errorf("%s has no forms", spec.ID)
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
	for _, spec := range commandSpecs {
		if spec.legacyBridge != legacyCommandSpecInventory[spec.ID] {
			t.Errorf("%s bridge state and inventory differ", spec.ID)
		}
	}
	for id := range legacyCommandSpecInventory {
		if !ids[id] {
			t.Errorf("bridge inventory contains unknown ID %s", id)
		}
	}
	for i := range commands {
		if commands[i].spec == nil || commands[i].surfaceSpec == nil {
			t.Errorf("command %q is not spec-bound", commands[i].name)
		}
	}
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
	}
	flags, operands, err := parseCommandFlags(specs, []string{
		"--format=html", "--open=false", "notebook", "--unknown", "-f", "text", "--", "--literal",
	})
	if err != nil {
		t.Fatal(err)
	}
	wantFlags := map[string][]string{
		"format": {"html", "text"},
		"open":   {"false"},
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
