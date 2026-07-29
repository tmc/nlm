package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	updateCommandDocsEnv = "UPDATE_COMMAND_DOCS"
	commandDocsOutputEnv = "COMMAND_DOCS_OUTPUT"
	commandDocsBegin     = "<!-- BEGIN GENERATED COMMAND SIGNATURES -->"
	commandDocsEnd       = "<!-- END GENERATED COMMAND SIGNATURES -->"
)

func TestCommandReferenceSignatures(t *testing.T) {
	name := os.Getenv(commandDocsOutputEnv)
	if name == "" {
		name = filepath.Join("..", "..", "docs", "commands.md")
	}
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	want := renderCommandReferenceSignatures()
	if os.Getenv(updateCommandDocsEnv) != "" {
		data, err = replaceCommandDocsRegion(data, want)
		if err != nil {
			t.Fatal(err)
		}
		if err := writeFileAtomic(name, data); err != nil {
			t.Fatal(err)
		}
	}
	got, err := commandDocsRegion(data)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("%s command signatures are stale; regenerate with %s=1", name, updateCommandDocsEnv)
	}
}

func renderCommandReferenceSignatures() string {
	var out strings.Builder
	for _, section := range helpSections {
		wroteSection := false
		for i := range commands {
			cmd := &commands[i]
			if cmd.section != section || cmd.surface != surfaceStable || cmd.hidden {
				continue
			}
			if !wroteSection {
				fmt.Fprintf(&out, "### %s\n\n", section)
				fmt.Fprintln(&out, "| Command | Description |")
				fmt.Fprintln(&out, "| --- | --- |")
				wroteSection = true
			}
			signature := "nlm " + cmd.name
			if cmd.argsUsage != "" {
				signature += " " + cmd.argsUsage
			}
			signature = strings.ReplaceAll(signature, "|", `\|`)
			summary := strings.ReplaceAll(cmd.usage, "|", `\|`)
			fmt.Fprintf(&out, "| `%s` | %s |\n", signature, summary)
		}
		if wroteSection {
			fmt.Fprintln(&out)
		}
	}
	return strings.TrimRight(out.String(), "\n")
}

func commandDocsRegion(data []byte) (string, error) {
	text := string(data)
	before, after, ok := strings.Cut(text, commandDocsBegin)
	if !ok {
		return "", errors.New("command docs begin marker is missing")
	}
	_ = before
	region, _, ok := strings.Cut(after, commandDocsEnd)
	if !ok {
		return "", errors.New("command docs end marker is missing")
	}
	return strings.Trim(region, "\n"), nil
}

func replaceCommandDocsRegion(data []byte, region string) ([]byte, error) {
	text := string(data)
	before, after, ok := strings.Cut(text, commandDocsBegin)
	if !ok {
		return nil, errors.New("command docs begin marker is missing")
	}
	_, after, ok = strings.Cut(after, commandDocsEnd)
	if !ok {
		return nil, errors.New("command docs end marker is missing")
	}
	return []byte(before + commandDocsBegin + "\n" + region + "\n" + commandDocsEnd + after), nil
}

func writeFileAtomic(name string, data []byte) error {
	info, err := os.Stat(name)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(name), "."+filepath.Base(name)+".*")
	if err != nil {
		return err
	}
	tmp := file.Name()
	defer os.Remove(tmp)
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Chmod(info.Mode()); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, name)
}
