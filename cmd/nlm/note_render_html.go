package main

import (
	"bytes"
	"fmt"
	"os"
)

func renderNoteHTMLToDestination(doc noteDocument, opts noteReadOptions) error {
	if opts.OutFile == "" {
		return renderNoteHTML(os.Stdout, doc)
	}
	var buf bytes.Buffer
	if err := renderNoteHTML(&buf, doc); err != nil {
		return err
	}
	if err := os.WriteFile(opts.OutFile, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("write html: %w", err)
	}
	fmt.Fprintf(os.Stderr, "nlm: wrote %s\n", opts.OutFile)
	if opts.Open {
		if err := openInBrowser(opts.OutFile); err != nil {
			fmt.Fprintf(os.Stderr, "nlm: could not open browser: %v\n", err)
		}
	}
	return nil
}
