package main

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/tmc/nlm/notebooklm"
)

var uuidRE = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func resolveLabelArg(c *notebooklm.Client, notebookID, arg string) (string, error) {
	if uuidRE.MatchString(arg) {
		return arg, nil
	}
	labels, err := c.GetLabels(context.Background(), notebookID)
	if err != nil {
		return "", fmt.Errorf("list labels to resolve %q: %w", arg, err)
	}
	want := strings.ToLower(arg)
	var matches []notebooklm.Label
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

func resolveSourceArg(c *notebooklm.Client, notebookID, arg string) (string, error) {
	if uuidRE.MatchString(arg) {
		return arg, nil
	}
	project, err := c.GetProject(context.Background(), notebookID)
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
