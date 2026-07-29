package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/tmc/nlm/internal/notebooklm/api"
)

func renderLabelList(out, status io.Writer, labels []api.Label, tty, jsonOutput bool) error {
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
