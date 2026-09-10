package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/tmc/nlm/notebooklm"
)

// renderLabelMembers lists one row per label member, so the source IDs that
// only --json carried are available in the human output too. titles may be
// nil, in which case the ID stands in for the title.
func renderLabelMembers(out, status io.Writer, labels []notebooklm.Label, titles map[string]string, tty bool) error {
	if tty {
		members := 0
		for _, l := range labels {
			members += len(l.SourceIDs)
		}
		fmt.Fprintf(status, "Total labels: %d, memberships: %d\n\n", len(labels), members)
		if members == 0 {
			fmt.Fprintln(status, "No labelled sources. Attach one with 'nlm label attach'.")
			return nil
		}
	}
	w := out
	flush := func() error { return nil }
	if f, ok := out.(*os.File); ok {
		w, flush = newListWriter(f)
	}
	fmt.Fprintln(w, "LABEL ID\tNAME\tSOURCE ID\tSOURCE")
	for _, l := range labels {
		for _, id := range l.SourceIDs {
			title := titles[id]
			if title == "" {
				title = id
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", l.LabelID, l.Name, id, title)
		}
	}
	return flush()
}

func renderLabelList(out, status io.Writer, labels []notebooklm.Label, tty, jsonOutput bool) error {
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
