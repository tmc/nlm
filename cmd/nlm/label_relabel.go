package main

import (
	"fmt"
	"os"
)

func printLabelUnlabeledUsage(cmdName string) {
	fmt.Fprintf(os.Stderr, "Usage: nlm %s <notebook-id>\n\n", cmdName)
	fmt.Fprintln(os.Stderr, "Apply existing labels to sources that don't yet belong to one (mode 0).")
	fmt.Fprintln(os.Stderr, "Cluster set is preserved; only unlabeled sources are touched.")
}

func printLabelRelabelAllUsage(cmdName string) {
	fmt.Fprintf(os.Stderr, "Usage: nlm %s <notebook-id>\n\n", cmdName)
	fmt.Fprintln(os.Stderr, "Trigger a full re-cluster of the notebook (mode 1) — the UI's \"Relabel all\".")
	fmt.Fprintln(os.Stderr, "On large notebooks this can hit the 60s server deadline (exit-class=transient).")
}
