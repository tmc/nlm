package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/tmc/nlm/internal/notebooklm/api"
)

func printLabelAttachUsage(cmdName string) {
	fmt.Fprintf(os.Stderr, "Usage: nlm %s <notebook-id> <label-id|name> <source-id|name>\n\n", cmdName)
	fmt.Fprintln(os.Stderr, "Attach a source to an existing label. Either argument may be a UUID or a")
	fmt.Fprintln(os.Stderr, "name; names are resolved case-insensitively against the notebook's labels")
	fmt.Fprintln(os.Stderr, "and sources, and must match exactly one entry. Only the single-source form")
	fmt.Fprintln(os.Stderr, "is HAR-verified — invoke once per source for now.")
}

func validateLabelAttachArgs(cmdName string, args []string) error {
	if len(args) != 3 {
		fmt.Fprintf(os.Stderr, "nlm: %s requires <notebook-id> <label-id|name> <source-id|name>\n\n", cmdName)
		printLabelAttachUsage(cmdName)
		return errBadArgs
	}
	return nil
}

var uuidRE = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func runLabelAttach(c *api.Client, args []string) error {
	notebookID, labelArg, sourceArg := args[0], args[1], args[2]

	labelID, err := resolveLabelArg(c, notebookID, labelArg)
	if err != nil {
		return err
	}
	sourceID, err := resolveSourceArg(c, notebookID, sourceArg)
	if err != nil {
		return err
	}

	if err := c.AttachLabelSource(notebookID, labelID, sourceID); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Attached source %s to label %s\n", sourceID, labelID)
	return nil
}

func resolveLabelArg(c *api.Client, notebookID, arg string) (string, error) {
	if uuidRE.MatchString(arg) {
		return arg, nil
	}
	labels, err := c.GetLabels(notebookID)
	if err != nil {
		return "", fmt.Errorf("list labels to resolve %q: %w", arg, err)
	}
	want := strings.ToLower(arg)
	var matches []api.Label
	for _, l := range labels {
		if strings.ToLower(l.Name) == want {
			matches = append(matches, l)
		}
	}
	switch len(matches) {
	case 0:
		return "", fmt.Errorf("no label named %q in notebook (use 'nlm label list' to see options)", arg)
	case 1:
		return matches[0].LabelID, nil
	default:
		return "", fmt.Errorf("label name %q is ambiguous (%d matches); pass the label ID instead", arg, len(matches))
	}
}

func resolveSourceArg(c *api.Client, notebookID, arg string) (string, error) {
	if uuidRE.MatchString(arg) {
		return arg, nil
	}
	project, err := c.GetProject(notebookID)
	if err != nil {
		return "", fmt.Errorf("list sources to resolve %q: %w", arg, err)
	}
	want := strings.ToLower(arg)
	var matches []string
	for _, src := range project.Sources {
		if strings.ToLower(strings.TrimSpace(src.Title)) == want {
			matches = append(matches, src.SourceId.GetSourceId())
		}
	}
	switch len(matches) {
	case 0:
		return "", fmt.Errorf("no source titled %q in notebook (use 'nlm source list' to see options)", arg)
	case 1:
		return matches[0], nil
	default:
		return "", fmt.Errorf("source title %q is ambiguous (%d matches); pass the source ID instead", arg, len(matches))
	}
}
