package main

import (
	"fmt"
)

type noteReadOptions struct {
	Format  string
	OutFile string
	Open    bool
}

func validateNoteFormat(opts *noteReadOptions) error {
	switch opts.Format {
	case "", "text":
		opts.Format = "text"
	case "markdown", "md":
		opts.Format = "markdown"
	case "html":
		opts.Format = "html"
	default:
		return fmt.Errorf("unknown --format %q (want text, markdown, or html)", opts.Format)
	}
	if opts.Format != "html" {
		if opts.OutFile != "" {
			return fmt.Errorf("--out only applies to --format=html")
		}
		if opts.Open {
			return fmt.Errorf("--open only applies to --format=html")
		}
	}
	return nil
}
