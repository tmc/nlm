package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// renderChatHTMLToDestination renders doc to a self-contained HTML page.
// An explicit "-" writes to stdout. Otherwise the page is written to OutFile,
// or to the render cache when OutFile is empty.
func renderChatHTMLToDestination(doc chatDocument, ctx chatRenderContext, opts chatRenderOptions) error {
	path, err := chatHTMLDestination(doc.NotebookID, doc.ConversationID, opts.OutFile)
	if err != nil {
		return err
	}
	if path == "" {
		return renderChatHTML(os.Stdout, doc, ctx)
	}

	var buf bytes.Buffer
	if err := renderChatHTML(&buf, doc, ctx); err != nil {
		return err
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("write html: %w", err)
	}
	fmt.Fprintf(os.Stderr, "nlm: wrote %s\n", path)

	if opts.Open {
		if err := openInBrowser(path); err != nil {
			fmt.Fprintf(os.Stderr, "nlm: could not open browser: %v\n", err)
		}
	}
	return nil
}

// chatHTMLDestination returns the output path for an HTML conversation render.
// An empty path means stdout.
func chatHTMLDestination(notebookID, conversationID, outFile string) (string, error) {
	if outFile == "-" {
		return "", nil
	}
	if outFile != "" {
		return outFile, nil
	}
	dir, err := renderCacheDir()
	if err != nil {
		return "", fmt.Errorf("create render cache: %w", err)
	}
	dir = filepath.Join(dir, notebookID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create render directory: %w", err)
	}
	return filepath.Join(dir, conversationID+".html"), nil
}

// openInBrowser opens path with the platform's default handler. A failure is
// the caller's to surface as a warning; opening is a convenience, never a
// prerequisite of a successful render.
func openInBrowser(path string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", path)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", path)
	default: // linux and other unix
		cmd = exec.Command("xdg-open", path)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	return nil
}
