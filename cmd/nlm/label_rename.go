package main

import (
	"fmt"
	"os"
)

func printLabelRenameUsage(cmdName string) {
	fmt.Fprintf(os.Stderr, "Usage: nlm %s <notebook-id> <label-id> <new-name>\n\n", cmdName)
	fmt.Fprintln(os.Stderr, "Rename an existing label. Use 'nlm label list' to find the label ID.")
}
