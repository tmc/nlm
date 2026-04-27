package main

import (
	"fmt"
	"os"

	"github.com/tmc/nlm/internal/notebooklm/api"
)

func printLabelUnlabeledUsage(cmdName string) {
	fmt.Fprintf(os.Stderr, "Usage: nlm %s <notebook-id>\n\n", cmdName)
	fmt.Fprintln(os.Stderr, "Apply existing labels to sources that don't yet belong to one (mode 0).")
	fmt.Fprintln(os.Stderr, "Cluster set is preserved; only unlabeled sources are touched.")
}

func validateLabelUnlabeledArgs(cmdName string, args []string) error {
	if len(args) != 1 {
		fmt.Fprintf(os.Stderr, "nlm: %s requires exactly one argument: <notebook-id>\n\n", cmdName)
		printLabelUnlabeledUsage(cmdName)
		return errBadArgs
	}
	return nil
}

func runLabelUnlabeled(c *api.Client, args []string) error {
	labels, err := c.LabelUnlabeled(args[0])
	if err != nil {
		return err
	}
	return renderLabelList(os.Stdout, os.Stderr, labels, isTerminal(os.Stdout))
}

func printLabelRelabelAllUsage(cmdName string) {
	fmt.Fprintf(os.Stderr, "Usage: nlm %s <notebook-id>\n\n", cmdName)
	fmt.Fprintln(os.Stderr, "Trigger a full re-cluster of the notebook (mode 1) — the UI's \"Relabel all\".")
	fmt.Fprintln(os.Stderr, "On large notebooks this can hit the 60s server deadline (exit-class=transient).")
}

func validateLabelRelabelAllArgs(cmdName string, args []string) error {
	if len(args) != 1 {
		fmt.Fprintf(os.Stderr, "nlm: %s requires exactly one argument: <notebook-id>\n\n", cmdName)
		printLabelRelabelAllUsage(cmdName)
		return errBadArgs
	}
	return nil
}

func runLabelRelabelAll(c *api.Client, args []string) error {
	labels, err := c.RelabelAll(args[0])
	if err != nil {
		return err
	}
	return renderLabelList(os.Stdout, os.Stderr, labels, isTerminal(os.Stdout))
}
