package main

import (
	"fmt"
	"os"
)

func printLabelEmojiUsage(cmdName string) {
	fmt.Fprintf(os.Stderr, "Usage: nlm %s <notebook-id> <label-id> <emoji>\n\n", cmdName)
	fmt.Fprintln(os.Stderr, "Set the emoji on an existing label. Pass an empty string to clear it.")
}
