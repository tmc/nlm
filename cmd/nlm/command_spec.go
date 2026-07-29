package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/tmc/nlm/internal/notebooklm/api"
)

//go:generate env UPDATE_COMMAND_DOCS=1 go test -run ^TestCommandReferenceSignatures$

// commandID is the stable identity of one command behavior. A command may
// expose that behavior through several stable or compatibility surfaces.
type commandID string

// commandSpec describes one command behavior and every surface that reaches
// it. Parsing, routing, help, and command-reference signatures all use this
// specification.
type commandSpec struct {
	ID       commandID
	Section  string
	Summary  string
	Flags    []flagSpec
	Forms    []commandForm
	Surfaces []commandSurfaceSpec
	Decode   func(parsedCommand) (commandCall, error)

	FlagGroup      string
	FlagGroupAfter int

	IgnoredArguments    []string
	DeferFlagErrors     bool
	DeferFlagValidation bool

	aliases   []string
	noAuth    bool // command does not require authentication
	noClient  bool // command does not need an API client
	directRPC bool // command requires direct RPC mode
	hidden    bool // base surface is omitted from help
	parse     func(*commandSurfaceSpec, []string, globalOptions) (parsedCommand, error)
}

// commandSurfaceSpec describes one user-visible route to a command behavior.
// Nil Forms means the surface uses commandSpec.Forms.
type commandSurfaceSpec struct {
	Path        []string
	Surface     commandSurface
	Forms       []commandForm
	Adapt       func(parsedCommand) (parsedCommand, error)
	Replacement []string
	Help        commandHelpSpec

	Aliases [][]string
	Section string
}

type commandHelpSpec struct {
	UsageTitle string
	Body       string
}

// commandForm is one executable operand grammar.
type commandForm struct {
	Parts       []operandSpec
	Constraints []constraint
	Hidden      bool
}

// operandSpec describes one literal or named operand in a command form.
type operandSpec struct {
	Name        string
	Placeholder string
	Usage       string
	Cardinality cardinality
	Literal     string
	Hidden      bool
	Virtual     bool
}

type cardinality uint8

const (
	cardinalityRequired cardinality = iota
	cardinalityOptional
	cardinalityOneOrMore
	cardinalityZeroOrMore
)

// flagSpec describes one command-owned flag. Value is empty for a boolean
// flag and names the value placeholder otherwise. OptionalValue permits a
// bare flag or an explicit value joined with "=".
type flagSpec struct {
	Name          string
	Aliases       []string
	Value         string
	OptionalValue bool
	Description   string
	Visibility    flagVisibility
	Inline        bool
}

type flagVisibility uint8

const (
	flagVisible flagVisibility = iota
	flagHidden
	flagDeprecated
)

func commandSynopsis(spec *commandSpec, surface *commandSurfaceSpec) string {
	forms := spec.Forms
	if len(surface.Forms) > 0 {
		forms = surface.Forms
	}
	var rendered []string
	for _, form := range forms {
		if form.Hidden {
			continue
		}
		rendered = append(rendered, renderCommandForm(spec, form))
	}
	return strings.Join(rendered, " | ")
}

func renderCommandForm(spec *commandSpec, form commandForm) string {
	var before, after []string
	groupAfter := spec.FlagGroupAfter
	visible := 0
	for _, part := range form.Parts {
		if part.Hidden {
			continue
		}
		rendered := renderOperand(part)
		if rendered == "" {
			continue
		}
		if visible < groupAfter {
			before = append(before, rendered)
		} else {
			after = append(after, rendered)
		}
		visible++
	}

	var flags []string
	grouped := false
	for _, flag := range spec.Flags {
		if flag.Visibility != flagVisible {
			continue
		}
		if !flag.Inline {
			grouped = true
			continue
		}
		label := "--" + flag.Name
		if flag.Value != "" {
			label += " " + flag.Value
		}
		flags = append(flags, "["+label+"]")
	}
	if grouped {
		group := spec.FlagGroup
		if group == "" {
			group = "flags"
		}
		flags = append([]string{"[" + group + "]"}, flags...)
	}
	return strings.Join(append(append(before, flags...), after...), " ")
}

func renderOperand(spec operandSpec) string {
	if spec.Usage != "" {
		return spec.Usage
	}
	if spec.Literal != "" {
		return spec.Literal
	}
	placeholder := spec.Placeholder
	if placeholder == "" {
		placeholder = spec.Name
	}
	switch spec.Cardinality {
	case cardinalityRequired:
		return "<" + placeholder + ">"
	case cardinalityOptional:
		return "[" + placeholder + "]"
	case cardinalityOneOrMore:
		return "<" + placeholder + ">"
	case cardinalityZeroOrMore:
		return "[" + placeholder + "...]"
	default:
		return ""
	}
}

// parsedCommand is the stringly grammar result passed to a command decoder.
// Decode immediately converts it into a named argument type or an existing
// typed custom-parser result.
type parsedCommand struct {
	Form  int
	Args  map[string][]string
	Flags map[string][]string

	path    string
	globals globalOptions
	raw     []string

	flagOccurrences []parsedFlag
	flagError       error
}

type parsedFlag struct {
	Name      string
	InputName string
	Value     string
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
	if spec.Virtual {
		return matchOperandParts(parts, args, part+1, arg, values)
	}
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
// flags remain operands to preserve the Phase 1 command grammar.
func parseCommandFlags(specs []flagSpec, args []string) (map[string][]string, []string, error) {
	flags, operands, _, err := parseCommandFlagsDetailed(specs, args, false)
	return flags, operands, err
}

func parseCommandFlagsDetailed(
	specs []flagSpec,
	args []string,
	deferValidation bool,
) (map[string][]string, []string, []parsedFlag, error) {
	byName := make(map[string]flagSpec)
	for _, spec := range specs {
		if spec.Name == "" {
			return nil, nil, nil, fmt.Errorf("command flag has empty name")
		}
		byName[spec.Name] = spec
		for _, alias := range spec.Aliases {
			byName[alias] = spec
		}
	}

	flags := make(map[string][]string)
	var operands []string
	var occurrences []parsedFlag
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
		if flag.OptionalValue {
			if !hasValue {
				value = "true"
			}
			flags[flag.Name] = append(flags[flag.Name], value)
			occurrences = append(occurrences, parsedFlag{
				Name:      flag.Name,
				InputName: name,
				Value:     value,
			})
			continue
		}
		if flag.Value == "" {
			if hasValue {
				if !deferValidation {
					if _, err := strconv.ParseBool(value); err != nil {
						return flags, operands, occurrences, fmt.Errorf("invalid boolean value %q for -%s: parse error", value, name)
					}
				}
			} else {
				value = "true"
			}
			flags[flag.Name] = append(flags[flag.Name], value)
			occurrences = append(occurrences, parsedFlag{
				Name:      flag.Name,
				InputName: name,
				Value:     value,
			})
			continue
		}
		if !hasValue {
			if i+1 >= len(args) {
				return flags, operands, occurrences, fmt.Errorf("flag needs an argument: %s", arg)
			}
			i++
			value = args[i]
		}
		flags[flag.Name] = append(flags[flag.Name], value)
		occurrences = append(occurrences, parsedFlag{
			Name:      flag.Name,
			InputName: name,
			Value:     value,
		})
	}
	return flags, operands, occurrences, nil
}

// parseCommandSpec selects a surface form, validates its constraints, and
// applies the surface adapter.
func parseCommandSpec(spec *commandSpec, surface *commandSurfaceSpec, args []string, globals globalOptions) (parsedCommand, error) {
	flags := make(map[string][]string)
	raw := append([]string(nil), args...)
	parseArgs := omitCommandArguments(args, spec.IgnoredArguments)
	operands := parseArgs
	var occurrences []parsedFlag
	var flagError error
	if len(spec.Flags) > 0 {
		flags, operands, occurrences, flagError = parseCommandFlagsDetailed(
			spec.Flags,
			parseArgs,
			spec.DeferFlagValidation,
		)
		if flagError != nil && !spec.DeferFlagErrors {
			return parsedCommand{}, flagError
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
		parsed.raw = raw
		parsed.flagOccurrences = occurrences
		parsed.flagError = flagError
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

func omitCommandArguments(args, ignored []string) []string {
	if len(ignored) == 0 {
		return args
	}
	ignore := make(map[string]bool, len(ignored))
	for _, arg := range ignored {
		ignore[arg] = true
	}
	out := make([]string, 0, len(args))
	for _, arg := range args {
		if !ignore[arg] {
			out = append(out, arg)
		}
	}
	return out
}
