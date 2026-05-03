package main

import (
	"fmt"
	"slices"

	"github.com/tmc/nlm/internal/notebooklm/api"
)

type labelReader interface {
	GetLabels(projectID string) ([]api.Label, error)
}

type labelAttacher interface {
	AttachLabelSource(projectID, labelID, sourceID string) error
}

// labelsForSource returns the IDs of labels currently attached to sourceID.
// An empty result (with no error) means the source has no label assignments.
func labelsForSource(c labelReader, notebookID, sourceID string) ([]string, error) {
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
func attachLabels(c labelAttacher, notebookID, newSourceID string, labelIDs []string) error {
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

func attachLabelsToSources(c labelAttacher, notebookID string, sourceIDs, labelIDs []string) error {
	var failed []string
	for _, sid := range sourceIDs {
		for _, lid := range labelIDs {
			if err := c.AttachLabelSource(notebookID, lid, sid); err != nil {
				failed = append(failed, fmt.Sprintf("%s/%s: %v", lid, sid, err))
			}
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("attach %d/%d label assignments: %v", len(failed), len(sourceIDs)*len(labelIDs), failed)
	}
	return nil
}
