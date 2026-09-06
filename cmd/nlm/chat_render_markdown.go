package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/tmc/nlm/internal/richrender"
)

func prepareChatTemplate(opts *chatRenderOptions) error {
	if opts.TemplateFile == "" {
		return nil
	}
	if opts.TemplateFile == "-" {
		return fmt.Errorf("--template requires a file path, not stdin")
	}
	data, err := os.ReadFile(opts.TemplateFile)
	if err != nil {
		return fmt.Errorf("read template: %w", err)
	}
	opts.Template, err = richrender.ParseMarkdownTemplate(opts.TemplateFile, string(data))
	return err
}

func renderChatMarkdownToDestination(docs []chatDocument, ctx chatRenderContext, opts chatRenderOptions) error {
	var buf bytes.Buffer
	if opts.Template != nil {
		if err := opts.Template.Render(&buf, docs, ctx, opts.TemplateVars, resolveCitationMode(opts.CitationMode) != citationModeOff); err != nil {
			return err
		}
	} else {
		for i, doc := range docs {
			if i > 0 {
				buf.WriteString("\n---\n\n")
			}
			if err := renderChatMarkdown(&buf, doc, ctx); err != nil {
				return err
			}
		}
	}
	if opts.OutFile == "" || opts.OutFile == "-" {
		_, err := os.Stdout.Write(buf.Bytes())
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(opts.OutFile), ".nlm-report-*")
	if err != nil {
		return fmt.Errorf("create report: %w", err)
	}
	name := file.Name()
	defer os.Remove(name)
	if _, err := file.Write(buf.Bytes()); err != nil {
		file.Close()
		return fmt.Errorf("write report: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close report: %w", err)
	}
	if err := os.Rename(name, opts.OutFile); err != nil {
		return fmt.Errorf("replace report: %w", err)
	}
	fmt.Fprintf(os.Stderr, "nlm: wrote %s\n", opts.OutFile)
	return nil
}
