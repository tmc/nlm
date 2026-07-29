package main

import (
	"fmt"
	"os"
)

// printSourceSelectionUsage prints the source selector command help.
func printSourceSelectionUsage(cmdName string) {
	fmt.Fprintf(os.Stderr, "Usage: nlm %s [flags] <notebook-id> [source-id...]\n\n", cmdName)
	fmt.Fprintln(os.Stderr, "Flags:")
	fmt.Fprintln(os.Stderr, "  --source-ids <ids>       Focus on these source IDs ('a,b,c' or '-' for stdin)")
	fmt.Fprintln(os.Stderr, "  --source-match <regex>   Focus on sources whose title or UUID matches the regex")
	fmt.Fprintln(os.Stderr, "  --source-exclude <regex> Exclude sources whose title or UUID matches the regex")
	fmt.Fprintln(os.Stderr, "  --label-ids <ids>        Include sources tagged with any of these label IDs")
	fmt.Fprintln(os.Stderr, "  --label-match <regex>    Include sources tagged with any label whose name matches the regex")
	fmt.Fprintln(os.Stderr, "  --label-exclude <regex>  Exclude sources tagged with any label whose name matches the regex")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Examples:")
	fmt.Fprintf(os.Stderr, "  nlm %s <notebook-id> <source-id>\n", cmdName)
	fmt.Fprintf(os.Stderr, "  nlm %s --source-match '^spec/' <notebook-id>\n", cmdName)
	fmt.Fprintf(os.Stderr, "  nlm %s --source-match '^spec/' --source-exclude 'draft' <notebook-id>\n", cmdName)
	fmt.Fprintf(os.Stderr, "  nlm %s --label-match '^Testing$' <notebook-id>\n", cmdName)
}
