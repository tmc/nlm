package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/tmc/nlm/internal/notebooklm/api"
)

func printLabelListUsage(cmdName string) {
	fmt.Fprintf(os.Stderr, "Usage: nlm %s <notebook-id>\n\n", cmdName)
	fmt.Fprintln(os.Stderr, "List autolabel clusters (labels) for a notebook.")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Examples:")
	fmt.Fprintf(os.Stderr, "  nlm %s NOTEBOOK_ID\n", cmdName)
	fmt.Fprintf(os.Stderr, "  nlm --json %s NOTEBOOK_ID\n", cmdName)
}

func validateLabelListArgs(cmdName string, args []string) error {
	if len(args) != 1 {
		fmt.Fprintf(os.Stderr, "nlm: %s requires exactly one argument: <notebook-id>\n\n", cmdName)
		printLabelListUsage(cmdName)
		return errBadArgs
	}
	return nil
}

func runLabelList(c *api.Client, args []string) error {
	labels, err := c.GetLabels(args[0])
	if err != nil {
		return err
	}
	return renderLabelList(os.Stdout, os.Stderr, labels, isTerminal(os.Stdout))
}

func renderLabelList(out, status io.Writer, labels []api.Label, tty bool) error {
	if jsonOutput {
		enc := json.NewEncoder(out)
		for _, l := range labels {
			rec := labelListRecord{
				LabelID:     l.LabelID,
				Name:        l.Name,
				SourceCount: len(l.SourceIDs),
				SourceIDs:   l.SourceIDs,
			}
			if err := enc.Encode(rec); err != nil {
				return err
			}
		}
		return nil
	}

	if tty {
		fmt.Fprintf(status, "Total labels: %d\n\n", len(labels))
		if len(labels) == 0 {
			fmt.Fprintln(status, "No labels found. The notebook may not have run autolabel yet.")
			return nil
		}
	}

	w := out
	flush := func() error { return nil }
	if f, ok := out.(*os.File); ok {
		w, flush = newListWriter(f)
	}
	fmt.Fprintln(w, "LABEL ID\tNAME\tSOURCES")
	for _, l := range labels {
		fmt.Fprintf(w, "%s\t%s\t%d\n", l.LabelID, l.Name, len(l.SourceIDs))
	}
	return flush()
}
