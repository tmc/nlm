package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/tmc/nlm/internal/notebooklm/api"
)

type deckDownloadOptions struct {
	ArtifactID string
	Format     string
	Output     string
}

// runDeckDownload fetches the rendered deck file with an authenticated client.
// When the direct fetch is blocked by the usercontent host's browser-auth
// requirement, it prints the signed download URL so the user can open it in a
// logged-in browser; while the deck is still generating it prints the notebook
// URL instead.
func runDeckDownload(c *api.Client, args deckDownloadArgs) error {
	output := args.Options.Output
	if output == "" {
		output = "deck." + args.Options.Format
	}

	derr := c.DownloadArtifactFile(context.Background(), args.Options.ArtifactID, args.Options.Format, output)
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
		fmt.Fprintf(os.Stderr, "Slide deck %s is still generating; no %s file yet.\n", args.Options.ArtifactID, args.Options.Format)
		fmt.Println(notebookBrowserURL(args.NotebookID))
		return fmt.Errorf("download slide deck: %w", derr)
	}

	// The file exists but the direct fetch failed (the usercontent host needs a
	// browser auth context). Print the signed download URL on stdout so the user
	// can open it in their logged-in browser — strictly more useful than the
	// notebook URL.
	if u, uerr := c.ArtifactDownloadURLForFormat(context.Background(), args.Options.ArtifactID, args.Options.Format); uerr == nil {
		fmt.Fprintf(os.Stderr, "Direct download failed (%v); the file requires a browser session.\n", derr)
		fmt.Fprintf(os.Stderr, "Open this %s link while logged in to NotebookLM:\n", args.Options.Format)
		fmt.Println(u)
		return fmt.Errorf("download slide deck: direct fetch unavailable; signed URL printed above")
	}

	// Couldn't even resolve a URL for the requested format.
	fmt.Fprintf(os.Stderr, "Could not download %s for artifact %s: %v\n", args.Options.Format, args.Options.ArtifactID, derr)
	fmt.Println(notebookBrowserURL(args.NotebookID))
	return fmt.Errorf("download slide deck: %w", derr)
}
