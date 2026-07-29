package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/tmc/nlm/internal/notebooklm/api"
)

type notebookListOptions struct {
	All   bool
	Limit int // -1 means use the default TTY/piped behavior
	JSON  bool
}

func list(c *api.Client, opts notebookListOptions) error {
	notebooks, err := c.ListRecentlyViewedProjects(context.Background())
	if err != nil {
		return err
	}
	return renderNotebookList(os.Stdout, os.Stderr, notebooks, opts, isTerminal(os.Stdout))
}

func renderNotebookList(out io.Writer, status io.Writer, notebooks []*api.Notebook, opts notebookListOptions, tty bool) error {
	total := len(notebooks)
	limit := notebookListLimit(total, opts, tty)

	if opts.JSON {
		enc := json.NewEncoder(out)
		for i := 0; i < limit; i++ {
			nb := notebooks[i]
			rec := notebookListRecord{
				NotebookID:  nb.ProjectId,
				Title:       nb.Title,
				Emoji:       strings.TrimSpace(nb.Emoji),
				SourceCount: len(nb.Sources),
			}
			ts := nb.GetMetadata().GetModifiedTime()
			if ts == nil {
				ts = nb.GetMetadata().GetCreateTime()
			}
			if ts != nil {
				rec.LastUpdated = ts.AsTime().Format(time.RFC3339)
			}
			if err := enc.Encode(rec); err != nil {
				return err
			}
		}
		return nil
	}

	if tty {
		switch {
		case total == 0:
			fmt.Fprintln(status, "Total notebooks: 0")
			fmt.Fprintln(status)
		case limit >= total:
			fmt.Fprintf(status, "Total notebooks: %d (showing all)\n\n", total)
		default:
			fmt.Fprintf(status, "Total notebooks: %d (showing first %d)\n\n", total, limit)
		}
	}

	w := out
	flush := func() error { return nil }
	if f, ok := out.(*os.File); ok {
		w, flush = newListWriter(f)
	}
	fmt.Fprintln(w, "ID\tTITLE\tSOURCES\tLAST UPDATED")
	for i := 0; i < limit; i++ {
		nb := notebooks[i]
		title := nb.Title
		if tty {
			if emoji := strings.TrimSpace(nb.Emoji); emoji != "" {
				title = emoji + " \b" + nb.Title
			}
			if len(title) > 45 {
				title = title[:42] + "..."
			}
		}
		sourceCount := len(nb.Sources)
		var updated string
		ts := nb.GetMetadata().GetModifiedTime()
		if ts == nil {
			ts = nb.GetMetadata().GetCreateTime()
		}
		if ts != nil {
			updated = ts.AsTime().Format(time.RFC3339)
		}
		fmt.Fprintf(w, "%s\t%s\t%d\t%s\n", nb.ProjectId, title, sourceCount, updated)
	}
	return flush()
}

func notebookListLimit(total int, opts notebookListOptions, tty bool) int {
	switch {
	case opts.Limit > 0 && opts.Limit < total:
		return opts.Limit
	case opts.Limit > 0:
		return total
	case tty && !opts.All && total > 10:
		return 10
	default:
		return total
	}
}
