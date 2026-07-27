package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/tmc/nlm/internal/notebooklm/api"
)

type deckDownloadOptions struct {
	ArtifactID string
	Format     string
	Output     string
}

func printDeckDownloadUsage(cmdName string) {
	fmt.Fprintf(os.Stderr, "Usage: nlm %s <notebook-id> --id <artifact-id> [--format pdf|pptx] [--output file]\n\n", cmdName)
	fmt.Fprintln(os.Stderr, "Downloads a rendered slide deck (PDF or PPTX) for a completed deck artifact.")
	fmt.Fprintln(os.Stderr, "If the deck is still generating or the rendered file is unavailable, it falls")
	fmt.Fprintln(os.Stderr, "back to printing the NotebookLM browser URL.")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Flags:")
	fmt.Fprintln(os.Stderr, "  --id, --artifact-id <id>  Slide deck artifact ID")
	fmt.Fprintln(os.Stderr, "  --format, -f <format>     Export format: pdf (default) or pptx")
	fmt.Fprintln(os.Stderr, "  --output, -o <file>       Output filename (default deck.<format>)")
}

func validateDeckDownloadArgsWithOptions(cmdName string, args []string, _ globalOptions) error {
	if _, _, err := parseDeckDownloadArgs(args); err != nil {
		fmt.Fprintf(os.Stderr, "nlm: %s: %v\n\n", cmdName, err)
		printDeckDownloadUsage(cmdName)
		return errBadArgs
	}
	return nil
}

func parseDeckDownloadArgs(args []string) (deckDownloadOptions, string, error) {
	opts := deckDownloadOptions{Format: "pdf"}
	flags := flag.NewFlagSet("deck-download", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&opts.ArtifactID, "id", "", "")
	flags.StringVar(&opts.ArtifactID, "artifact-id", "", "")
	flags.StringVar(&opts.Format, "format", opts.Format, "")
	flags.StringVar(&opts.Format, "f", opts.Format, "")
	flags.StringVar(&opts.Output, "output", "", "")
	flags.StringVar(&opts.Output, "o", "", "")

	flagArgs, positional, err := splitCommandFlags(args, map[string]bool{
		"id":          true,
		"artifact-id": true,
		"format":      true,
		"f":           true,
		"output":      true,
		"o":           true,
	}, nil)
	if err != nil {
		return opts, "", err
	}
	if err := flags.Parse(flagArgs); err != nil {
		return opts, "", err
	}
	if len(positional) != 1 {
		return opts, "", fmt.Errorf("requires exactly one notebook id")
	}
	if opts.ArtifactID == "" {
		return opts, "", fmt.Errorf("missing --id <artifact-id>")
	}
	switch opts.Format {
	case "pdf", "pptx":
	default:
		return opts, "", fmt.Errorf("unsupported format %q (want pdf or pptx)", opts.Format)
	}
	return opts, positional[0], nil
}

// runDeckDownload fetches the rendered deck file with an authenticated client.
// When the direct fetch is blocked by the usercontent host's browser-auth
// requirement, it prints the signed download URL so the user can open it in a
// logged-in browser; while the deck is still generating it prints the notebook
// URL instead.
func runDeckDownload(c *api.Client, args []string) error {
	opts, notebookID, err := parseDeckDownloadArgs(args)
	if err != nil {
		return err
	}
	output := opts.Output
	if output == "" {
		output = "deck." + opts.Format
	}

	derr := c.DownloadArtifactFile(context.Background(), opts.ArtifactID, opts.Format, output)
	if derr == nil {
		fmt.Println(output)
		fmt.Fprintf(os.Stderr, "Saved slide deck to %s\n", output)
		if stat, err := os.Stat(output); err == nil {
			fmt.Fprintf(os.Stderr, "  File size: %.2f MB\n", float64(stat.Size())/(1024*1024))
		}
		return nil
	}

	// Still generating: nothing to download yet.
	if errors.Is(derr, api.ErrArtifactGenerating) {
		fmt.Fprintf(os.Stderr, "Slide deck %s is still generating; no %s file yet.\n", opts.ArtifactID, opts.Format)
		fmt.Println(notebookBrowserURL(notebookID))
		return fmt.Errorf("download slide deck: %w", derr)
	}

	// The file exists but the direct fetch failed (the usercontent host needs a
	// browser auth context). Print the signed download URL on stdout so the user
	// can open it in their logged-in browser — strictly more useful than the
	// notebook URL.
	if u, uerr := c.ArtifactDownloadURLForFormat(context.Background(), opts.ArtifactID, opts.Format); uerr == nil {
		fmt.Fprintf(os.Stderr, "Direct download failed (%v); the file requires a browser session.\n", derr)
		fmt.Fprintf(os.Stderr, "Open this %s link while logged in to NotebookLM:\n", opts.Format)
		fmt.Println(u)
		return fmt.Errorf("download slide deck: direct fetch unavailable; signed URL printed above")
	}

	// Couldn't even resolve a URL for the requested format.
	fmt.Fprintf(os.Stderr, "Could not download %s for artifact %s: %v\n", opts.Format, opts.ArtifactID, derr)
	fmt.Println(notebookBrowserURL(notebookID))
	return fmt.Errorf("download slide deck: %w", derr)
}
