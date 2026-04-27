package main

import (
	"fmt"
	"os"

	"github.com/tmc/nlm/internal/notebooklm/api"
)

func printLabelCreateUsage(cmdName string) {
	fmt.Fprintf(os.Stderr, "Usage: nlm %s <notebook-id> <name> [emoji]\n\n", cmdName)
	fmt.Fprintln(os.Stderr, "Create a new manual label on a notebook. The emoji is optional.")
	fmt.Fprintln(os.Stderr, "Returns the refreshed label list.")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Examples:")
	fmt.Fprintf(os.Stderr, "  nlm %s NOTEBOOK_ID \"Important\"\n", cmdName)
	fmt.Fprintf(os.Stderr, "  nlm %s NOTEBOOK_ID \"Bugs\" \"\\xf0\\x9f\\x90\\x9b\"\n", cmdName)
}

func validateLabelCreateArgs(cmdName string, args []string) error {
	if len(args) < 2 || len(args) > 3 {
		fmt.Fprintf(os.Stderr, "nlm: %s requires <notebook-id> <name> [emoji]\n\n", cmdName)
		printLabelCreateUsage(cmdName)
		return errBadArgs
	}
	return nil
}

func runLabelCreate(c *api.Client, args []string) error {
	emoji := ""
	if len(args) == 3 {
		emoji = args[2]
	}
	labels, err := c.CreateLabel(args[0], args[1], emoji)
	if err != nil {
		return err
	}
	return renderLabelList(os.Stdout, os.Stderr, labels, isTerminal(os.Stdout))
}
