package main

import (
	"fmt"
	"os"
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
