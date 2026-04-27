package main

import (
	"fmt"
	"os"

	"github.com/tmc/nlm/internal/notebooklm/api"
)

func printLabelRenameUsage(cmdName string) {
	fmt.Fprintf(os.Stderr, "Usage: nlm %s <notebook-id> <label-id> <new-name>\n\n", cmdName)
	fmt.Fprintln(os.Stderr, "Rename an existing label. Use 'nlm label list' to find the label ID.")
}

func validateLabelRenameArgs(cmdName string, args []string) error {
	if len(args) != 3 {
		fmt.Fprintf(os.Stderr, "nlm: %s requires <notebook-id> <label-id> <new-name>\n\n", cmdName)
		printLabelRenameUsage(cmdName)
		return errBadArgs
	}
	return nil
}

func runLabelRename(c *api.Client, args []string) error {
	if err := c.RenameLabel(args[0], args[1], args[2]); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Renamed %s to %q\n", args[1], args[2])
	return nil
}
