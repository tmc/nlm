package main

import (
	"fmt"
	"os"

	"github.com/tmc/nlm/internal/notebooklm/api"
)

func printLabelEmojiUsage(cmdName string) {
	fmt.Fprintf(os.Stderr, "Usage: nlm %s <notebook-id> <label-id> <emoji>\n\n", cmdName)
	fmt.Fprintln(os.Stderr, "Set the emoji on an existing label. Pass an empty string to clear it.")
}

func validateLabelEmojiArgs(cmdName string, args []string) error {
	if len(args) != 3 {
		fmt.Fprintf(os.Stderr, "nlm: %s requires <notebook-id> <label-id> <emoji>\n\n", cmdName)
		printLabelEmojiUsage(cmdName)
		return errBadArgs
	}
	return nil
}

func runLabelEmoji(c *api.Client, args []string) error {
	if err := c.SetLabelEmoji(args[0], args[1], args[2]); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Set emoji on %s\n", args[1])
	return nil
}
