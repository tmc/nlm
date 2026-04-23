package main

import (
	"fmt"
	"strings"
)

// splitCommandFlags extracts the flags recognized by a command-local parser
// while preserving the relative order of the remaining positional arguments.
// Unknown flags are left in the positional stream so compatibility behavior
// stays unchanged until the top-level parser is fully retired.
func splitCommandFlags(args []string, knownFlags, boolFlags map[string]bool) ([]string, []string, error) {
	flagArgs := make([]string, 0, len(args))
	positional := make([]string, 0, len(args))

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		if arg == "-" || !strings.HasPrefix(arg, "-") {
			positional = append(positional, arg)
			continue
		}

		name := strings.TrimLeft(arg, "-")
		if eq := strings.IndexByte(name, '='); eq >= 0 {
			name = name[:eq]
		}
		if !knownFlags[name] {
			positional = append(positional, arg)
			continue
		}

		flagArgs = append(flagArgs, arg)
		if boolFlags[name] || strings.Contains(arg, "=") {
			continue
		}
		if i+1 >= len(args) {
			return nil, nil, fmt.Errorf("flag needs an argument: %s", arg)
		}
		flagArgs = append(flagArgs, args[i+1])
		i++
	}

	return flagArgs, positional, nil
}
