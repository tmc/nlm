package main

import (
	"fmt"
	"os"
)

func printLabelDeleteUsage(cmdName string) {
	fmt.Fprintf(os.Stderr, "Usage: nlm %s <notebook-id> <label-id> [<label-id>...]\n\n", cmdName)
	fmt.Fprintln(os.Stderr, "Delete one or more labels by ID. Sources keep their content; only the cluster goes away.")
}
