package main

import (
	"fmt"
	"os"

	"github.com/tmc/nlm/internal/notebooklm/api"
)

func printLabelGenerateUsage(cmdName string) {
	fmt.Fprintf(os.Stderr, "Usage: nlm %s <notebook-id>\n\n", cmdName)
	fmt.Fprintln(os.Stderr, "Recompute autolabel clusters for a notebook (server-side clustering job).")
	fmt.Fprintln(os.Stderr, "Returns the freshly produced clusters in the same shape as 'label list'.")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Examples:")
	fmt.Fprintf(os.Stderr, "  nlm %s NOTEBOOK_ID\n", cmdName)
	fmt.Fprintf(os.Stderr, "  nlm --json %s NOTEBOOK_ID\n", cmdName)
}

func validateLabelGenerateArgs(cmdName string, args []string) error {
	if len(args) != 1 {
		fmt.Fprintf(os.Stderr, "nlm: %s requires exactly one argument: <notebook-id>\n\n", cmdName)
		printLabelGenerateUsage(cmdName)
		return errBadArgs
	}
	return nil
}

func runLabelGenerate(c *api.Client, args []string) error {
	labels, err := c.GenerateLabels(args[0])
	if err != nil {
		return err
	}
	return renderLabelList(os.Stdout, os.Stderr, labels, isTerminal(os.Stdout))
}
