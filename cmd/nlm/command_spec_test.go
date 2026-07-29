package main

import (
	"reflect"
	"testing"
)

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
		"--format=html", "--open", "notebook", "-f", "text", "--", "--literal",
	})
	if err != nil {
		t.Fatal(err)
	}
	wantFlags := map[string][]string{
		"format": {"html", "text"},
		"open":   {"true"},
	}
	if !reflect.DeepEqual(flags, wantFlags) {
		t.Errorf("flags = %#v, want %#v", flags, wantFlags)
	}
	wantOperands := []string{"notebook", "--literal"}
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
