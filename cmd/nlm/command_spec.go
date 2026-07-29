package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/tmc/nlm/internal/notebooklm/api"
)

// commandID is the stable identity of one command behavior. A command may
// expose that behavior through several stable or compatibility surfaces.
type commandID string

// commandSpec describes one command behavior and every surface that reaches
// it. Help remains on command during Phase 1 so this specification can become
// executable without changing presentation bytes.
type commandSpec struct {
	ID       commandID
	Section  string
	Summary  string
	Flags    []flagSpec
	Forms    []commandForm
	Surfaces []commandSurfaceSpec
	Decode   func(parsedCommand) (commandCall, error)

	definition   *commandDefinition
	parse        func(*commandSurfaceSpec, []string, globalOptions) (parsedCommand, error)
	legacyBridge bool
}

// commandSurfaceSpec describes one user-visible route to a command behavior.
// Nil Forms means the surface uses commandSpec.Forms.
type commandSurfaceSpec struct {
	Path        []string
	Surface     commandSurface
	Forms       []commandForm
	Adapt       func(parsedCommand) (parsedCommand, error)
	Replacement []string

	// Aliases and Section preserve current Phase 1 presentation. Phase 2 can
	// render aliases and sections directly from the specification.
	Aliases [][]string
	Section string
}

// commandForm is one executable operand grammar.
type commandForm struct {
	Parts       []operandSpec
	Constraints []constraint
}

// operandSpec describes one literal or named operand in a command form.
type operandSpec struct {
	Name        string
	Placeholder string
	Cardinality cardinality
	Literal     string
}

type cardinality uint8

const (
	cardinalityRequired cardinality = iota
	cardinalityOptional
	cardinalityOneOrMore
	cardinalityZeroOrMore
)

// flagSpec describes one command-owned flag. Value is empty for a boolean
// flag and names the value placeholder otherwise.
type flagSpec struct {
	Name        string
	Aliases     []string
	Value       string
	Description string
	Visibility  flagVisibility
}

type flagVisibility uint8

const (
	flagVisible flagVisibility = iota
	flagHidden
	flagDeprecated
)

// parsedCommand is the stringly grammar result passed to a command decoder.
// Decode immediately converts it into a named argument type or an existing
// typed custom-parser result.
type parsedCommand struct {
	Form  int
	Args  map[string][]string
	Flags map[string][]string

	path    string
	globals globalOptions
	legacy  legacyArgs
}

// legacyArgs is the temporary Phase 1 bridge for command behaviors not yet
// decoded by a commandSpec. legacyCommandSpecInventory tracks every remaining
// use; the bridge must be empty before Phase 1 is complete.
type legacyArgs struct {
	Values []string
}

// commandCall is a fully decoded command ready to run.
type commandCall func(context.Context, *api.Client) error

// constraint validates relationships that operand cardinality cannot express.
type constraint interface {
	Check(parsedCommand) error
}

type constraintFunc func(parsedCommand) error

func (f constraintFunc) Check(parsed parsedCommand) error {
	return f(parsed)
}

// parseCommandForm parses operands against one command form.
func parseCommandForm(form commandForm, args []string) (parsedCommand, bool) {
	parsed := parsedCommand{Args: make(map[string][]string)}
	if !matchOperandParts(form.Parts, args, 0, 0, parsed.Args) {
		return parsedCommand{}, false
	}
	return parsed, true
}

func matchOperandParts(parts []operandSpec, args []string, part, arg int, values map[string][]string) bool {
	if part == len(parts) {
		return arg == len(args)
	}
	spec := parts[part]
	minCount, maxCount := operandCountRange(spec.Cardinality, len(args)-arg)
	for count := maxCount; count >= minCount; count-- {
		if spec.Literal != "" {
			if count != 1 || arg >= len(args) || args[arg] != spec.Literal {
				continue
			}
		}
		old := len(values[spec.Name])
		if spec.Name != "" && spec.Literal == "" && count > 0 {
			values[spec.Name] = append(values[spec.Name], args[arg:arg+count]...)
		}
		if matchOperandParts(parts, args, part+1, arg+count, values) {
			return true
		}
		if spec.Name != "" {
			values[spec.Name] = values[spec.Name][:old]
			if len(values[spec.Name]) == 0 {
				delete(values, spec.Name)
			}
		}
	}
	return false
}

func operandCountRange(cardinality cardinality, available int) (int, int) {
	switch cardinality {
	case cardinalityRequired:
		return 1, min(1, available)
	case cardinalityOptional:
		return 0, min(1, available)
	case cardinalityOneOrMore:
		return 1, available
	case cardinalityZeroOrMore:
		return 0, available
	default:
		return 1, 0
	}
}

// parseCommandFlags separates known command flags from operands. Unknown
// flags remain operands, matching splitCommandFlags during Phase 1.
func parseCommandFlags(specs []flagSpec, args []string) (map[string][]string, []string, error) {
	byName := make(map[string]flagSpec)
	for _, spec := range specs {
		if spec.Name == "" {
			return nil, nil, fmt.Errorf("command flag has empty name")
		}
		byName[spec.Name] = spec
		for _, alias := range spec.Aliases {
			byName[alias] = spec
		}
	}

	flags := make(map[string][]string)
	var operands []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			operands = append(operands, args[i+1:]...)
			break
		}
		if arg == "-" || !strings.HasPrefix(arg, "-") {
			operands = append(operands, arg)
			continue
		}
		nameValue := strings.TrimLeft(arg, "-")
		name, value, hasValue := nameValue, "", false
		if before, after, ok := strings.Cut(nameValue, "="); ok {
			name, value, hasValue = before, after, true
		}
		flag, ok := byName[name]
		if !ok {
			operands = append(operands, arg)
			continue
		}
		if flag.Value == "" {
			if hasValue {
				if _, err := strconv.ParseBool(value); err != nil {
					return nil, nil, fmt.Errorf("invalid boolean value %q for -%s: parse error", value, name)
				}
			} else {
				value = "true"
			}
			flags[flag.Name] = append(flags[flag.Name], value)
			continue
		}
		if !hasValue {
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("flag needs an argument: %s", arg)
			}
			i++
			value = args[i]
		}
		flags[flag.Name] = append(flags[flag.Name], value)
	}
	return flags, operands, nil
}

// parseCommandSpec selects a surface form, validates its constraints, and
// applies the surface adapter.
func parseCommandSpec(spec *commandSpec, surface *commandSurfaceSpec, args []string, globals globalOptions) (parsedCommand, error) {
	flags := make(map[string][]string)
	operands := args
	if len(spec.Flags) > 0 {
		var err error
		flags, operands, err = parseCommandFlags(spec.Flags, args)
		if err != nil {
			return parsedCommand{}, err
		}
	}
	forms := surface.Forms
	if forms == nil {
		forms = spec.Forms
	}
	var constraintErr error
	for i, form := range forms {
		parsed, ok := parseCommandForm(form, operands)
		if !ok {
			continue
		}
		parsed.Form = i
		parsed.Flags = flags
		parsed.path = strings.Join(surface.Path, " ")
		parsed.globals = globals
		valid := true
		for _, constraint := range form.Constraints {
			if err := constraint.Check(parsed); err != nil {
				constraintErr = err
				valid = false
				break
			}
		}
		if !valid {
			continue
		}
		if surface.Adapt != nil {
			return surface.Adapt(parsed)
		}
		return parsed, nil
	}
	if constraintErr != nil {
		return parsedCommand{}, constraintErr
	}
	return parsedCommand{}, errBadArgs
}
