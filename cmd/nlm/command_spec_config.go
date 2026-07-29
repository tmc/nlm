package main

import (
	"fmt"
	"strconv"
)

func configureTypedCommandSpec(
	spec *commandSpec,
	forms []commandForm,
	decode func(parsedCommand) (commandCall, error),
) {
	configureTypedCommandSpecWithErrorUsage(spec, forms, decode, func(path string, _ error) {
		printCommandUsageForPath(path)
	})
}

func configureTypedCommandSpecWithUsage(
	spec *commandSpec,
	forms []commandForm,
	decode func(parsedCommand) (commandCall, error),
	printUsageError func(string),
) {
	configureTypedCommandSpecWithErrorUsage(spec, forms, decode, func(path string, _ error) {
		printUsageError(path)
	})
}

func configureTypedCommandSpecWithErrorUsage(
	spec *commandSpec,
	forms []commandForm,
	decode func(parsedCommand) (commandCall, error),
	printUsageError func(string, error),
) {
	configureTypedCommandSpecWithParseError(spec, forms, decode, func(path string, err error) error {
		printUsageError(path, err)
		return errBadArgs
	})
}

func configureTypedCommandSpecWithParseError(
	spec *commandSpec,
	forms []commandForm,
	decode func(parsedCommand) (commandCall, error),
	handleParseError func(string, error) error,
) {
	spec.Forms = forms
	spec.parse = func(surface *commandSurfaceSpec, args []string, globals globalOptions) (parsedCommand, error) {
		parsed, err := parseCommandSpec(spec, surface, args, globals)
		if err != nil {
			return parsedCommand{}, handleParseError(parsedCommandPath(surface), err)
		}
		return parsed, nil
	}
	spec.Decode = decode
}

func parsedCommandPath(surface *commandSurfaceSpec) string {
	return joinCommandPath(surface.Path)
}

func joinCommandPath(path []string) string {
	if len(path) == 0 {
		return ""
	}
	out := path[0]
	for _, part := range path[1:] {
		out += " " + part
	}
	return out
}

func requiredOperand(name string) operandSpec {
	return operandSpec{
		Name:        name,
		Placeholder: operandPlaceholder(name),
		Cardinality: cardinalityRequired,
	}
}

func optionalOperand(name string) operandSpec {
	return operandSpec{
		Name:        name,
		Placeholder: operandPlaceholder(name),
		Cardinality: cardinalityOptional,
	}
}

func repeatedOperand(name string) operandSpec {
	return operandSpec{
		Name:        name,
		Placeholder: operandPlaceholder(name),
		Cardinality: cardinalityOneOrMore,
	}
}

func remainingOperand(name string) operandSpec {
	return operandSpec{
		Name:        name,
		Placeholder: operandPlaceholder(name),
		Cardinality: cardinalityZeroOrMore,
	}
}

func operandPlaceholder(name string) string {
	switch name {
	case "artifact", "conversation", "guidebook", "label", "note", "notebook", "share", "source":
		return name + "-id"
	case "image":
		return "image-path"
	case "preset":
		return "preset-id"
	default:
		return name
	}
}

func withPlaceholder(spec operandSpec, placeholder string) operandSpec {
	spec.Placeholder = placeholder
	return spec
}

func withUsage(spec operandSpec, usage string) operandSpec {
	spec.Usage = usage
	return spec
}

func hiddenOperand(spec operandSpec) operandSpec {
	spec.Hidden = true
	return spec
}

func virtualOperand(usage string) operandSpec {
	return operandSpec{Usage: usage, Virtual: true}
}

func commandFormOf(parts ...operandSpec) []commandForm {
	return []commandForm{{Parts: parts}}
}

func parsedArgument(parsed parsedCommand, name string) (string, error) {
	values := parsed.Args[name]
	if len(values) != 1 {
		return "", fmt.Errorf("decode %s: got %d values", name, len(values))
	}
	return values[0], nil
}

func parsedOptionalArgument(parsed parsedCommand, name string) (string, bool, error) {
	values := parsed.Args[name]
	switch len(values) {
	case 0:
		return "", false, nil
	case 1:
		return values[0], true, nil
	default:
		return "", false, fmt.Errorf("decode %s: got %d values", name, len(values))
	}
}

func parsedArguments(parsed parsedCommand, name string) ([]string, error) {
	values := parsed.Args[name]
	if len(values) == 0 {
		return nil, fmt.Errorf("decode %s: got no values", name)
	}
	return append([]string(nil), values...), nil
}

func parsedBoolFlag(parsed parsedCommand, name string, defaultValue bool) (bool, error) {
	values := parsed.Flags[name]
	if len(values) == 0 {
		return defaultValue, nil
	}
	value, err := strconv.ParseBool(values[len(values)-1])
	if err != nil {
		return false, fmt.Errorf("invalid boolean value %q for -%s: parse error", values[len(values)-1], name)
	}
	return value, nil
}

func parsedIntFlag(parsed parsedCommand, name string, defaultValue int) (int, error) {
	values := parsed.Flags[name]
	if len(values) == 0 {
		return defaultValue, nil
	}
	value, err := strconv.ParseInt(values[len(values)-1], 0, strconv.IntSize)
	if err != nil {
		return 0, fmt.Errorf("invalid value %q for flag -%s: parse error", values[len(values)-1], name)
	}
	return int(value), nil
}

func parsedStringFlag(parsed parsedCommand, name, defaultValue string) string {
	values := parsed.Flags[name]
	if len(values) == 0 {
		return defaultValue
	}
	return values[len(values)-1]
}
