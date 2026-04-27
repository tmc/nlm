package main

import (
	"fmt"
	"os"

	"github.com/tmc/nlm/internal/notebooklm/api"
)

func printLabelDeleteUsage(cmdName string) {
	fmt.Fprintf(os.Stderr, "Usage: nlm %s <notebook-id> <label-id> [<label-id>...]\n\n", cmdName)
	fmt.Fprintln(os.Stderr, "Delete one or more labels by ID. Sources keep their content; only the cluster goes away.")
}

func validateLabelDeleteArgs(cmdName string, args []string) error {
	if len(args) < 2 {
		fmt.Fprintf(os.Stderr, "nlm: %s requires <notebook-id> and at least one <label-id>\n\n", cmdName)
		printLabelDeleteUsage(cmdName)
		return errBadArgs
	}
	return nil
}

func runLabelDelete(c *api.Client, args []string) error {
	if err := c.DeleteLabels(args[0], args[1:]); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Deleted %d label(s)\n", len(args)-1)
	return nil
}
