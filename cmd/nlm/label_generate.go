package main

import (
	"fmt"
	"os"
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
