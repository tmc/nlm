package main

import (
	"fmt"
	"os"
)

func configureTypedCommandSpec(
	spec *commandSpec,
	forms []commandForm,
	decode func(parsedCommand) (commandCall, error),
) {
	configureTypedCommandSpecWithUsage(spec, forms, decode, func(path string) {
		fmt.Fprintf(os.Stderr, "usage: nlm %s %s\n", path, spec.definition.argsUsage)
	})
}

func configureTypedCommandSpecWithUsage(
	spec *commandSpec,
	forms []commandForm,
	decode func(parsedCommand) (commandCall, error),
	printUsageError func(string),
) {
	spec.Forms = forms
	spec.Flags = nil
	spec.parse = func(surface *commandSurfaceSpec, args []string, globals globalOptions) (parsedCommand, error) {
		parsed, err := parseCommandSpec(spec, surface, args, globals)
		if err != nil {
			printUsageError(parsedCommandPath(surface))
			return parsedCommand{}, errBadArgs
		}
		return parsed, nil
	}
	spec.Decode = decode
	spec.legacyBridge = false
	delete(legacyCommandSpecInventory, spec.ID)
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
		Placeholder: name,
		Cardinality: cardinalityRequired,
	}
}

func optionalOperand(name string) operandSpec {
	return operandSpec{
		Name:        name,
		Placeholder: name,
		Cardinality: cardinalityOptional,
	}
}

func repeatedOperand(name string) operandSpec {
	return operandSpec{
		Name:        name,
		Placeholder: name,
		Cardinality: cardinalityOneOrMore,
	}
}

func remainingOperand(name string) operandSpec {
	return operandSpec{
		Name:        name,
		Placeholder: name,
		Cardinality: cardinalityZeroOrMore,
	}
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
