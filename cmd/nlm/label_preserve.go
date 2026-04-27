package main

import (
	"fmt"
	"slices"

	"github.com/tmc/nlm/internal/notebooklm/api"
)

// labelsForSource returns the IDs of labels currently attached to sourceID.
// An empty result (with no error) means the source has no label assignments.
func labelsForSource(c *api.Client, notebookID, sourceID string) ([]string, error) {
	labels, err := c.GetLabels(notebookID)
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, l := range labels {
		if slices.Contains(l.SourceIDs, sourceID) {
			ids = append(ids, l.LabelID)
		}
	}
	return ids, nil
}

// attachLabels attaches newSourceID to each label in labelIDs. Failures are
// collected and returned together so callers can surface a partial-success
// warning without aborting the surrounding flow.
func attachLabels(c *api.Client, notebookID, newSourceID string, labelIDs []string) error {
	var failed []string
	for _, lid := range labelIDs {
		if err := c.AttachLabelSource(notebookID, lid, newSourceID); err != nil {
			failed = append(failed, fmt.Sprintf("%s: %v", lid, err))
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("attach %d/%d labels: %v", len(failed), len(labelIDs), failed)
	}
	return nil
}
