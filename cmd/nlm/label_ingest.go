package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/tmc/nlm/notebooklm"
)

// labelEnsurer is what an ingest command needs to turn a --label argument
// into a label ID: read the notebook's labels, and create one when asked.
type labelEnsurer interface {
	labelReader
	CreateLabel(ctx context.Context, projectID, name, emoji string) ([]notebooklm.Label, error)
}

// labelNotFoundError reports that no label in the notebook matched.
// Ambiguity is a different failure and must not be resolved by creating
// another label with the same name, so callers test for this type rather
// than for any resolution error.
type labelNotFoundError struct{ msg string }

func (e *labelNotFoundError) Error() string { return e.msg }

func labelNotFound(format string, args ...any) error {
	return &labelNotFoundError{msg: fmt.Sprintf(format, args...)}
}

// isLabelNotFound reports whether err is a no-such-label failure.
func isLabelNotFound(err error) bool {
	var e *labelNotFoundError
	return errors.As(err, &e)
}

// findLabel maps a label ID or name to a label ID. An argument that is
// already a known ID wins; otherwise names are matched case-insensitively
// and must be unambiguous.
func findLabel(labels []notebooklm.Label, arg string) (string, error) {
	want := strings.ToLower(arg)
	var matches []string
	for _, label := range labels {
		if label.LabelID == arg {
			return arg, nil
		}
		if strings.ToLower(label.Name) == want {
			matches = append(matches, label.LabelID)
		}
	}
	switch {
	case len(matches) == 1:
		return matches[0], nil
	case len(matches) > 1:
		return "", fmt.Errorf("label name %q is ambiguous (%d matches); pass the label ID instead", arg, len(matches))
	case uuidRE.MatchString(arg):
		return "", labelNotFound("no label %s in notebook (use 'nlm label list' to see options)", arg)
	default:
		return "", labelNotFound("no label named %q in notebook (use 'nlm label list' to see options)", arg)
	}
}

// resolveIngestLabel maps the --label argument of an ingest command to a
// label ID, creating the label first when create is set and no name matches.
// Ingest commands resolve before uploading anything, so a typo fails fast
// instead of after N sources have landed unlabeled.
//
// A UUID-shaped argument is never created: it is a label ID that does not
// exist, not a name.
func resolveIngestLabel(ctx context.Context, c labelEnsurer, notebookID, arg string, create bool) (string, error) {
	labels, err := c.GetLabels(ctx, notebookID)
	if err != nil {
		return "", fmt.Errorf("list labels: %w", err)
	}
	id, err := findLabel(labels, arg)
	if err == nil {
		return id, nil
	}
	if !isLabelNotFound(err) {
		return "", err
	}
	if !create {
		return "", fmt.Errorf("%w; pass --create-label to create it", err)
	}
	if uuidRE.MatchString(arg) {
		return "", err
	}
	// CreateLabel returns the refreshed list, so the new ID comes back
	// without a second read — GetLabels is eventually consistent and a read
	// straight after the write can still omit it.
	created, err := c.CreateLabel(ctx, notebookID, arg, "")
	if err != nil {
		return "", err
	}
	return findLabel(created, arg)
}
