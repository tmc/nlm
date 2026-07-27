package main

import (
	"context"
	"fmt"
	"slices"

	"github.com/tmc/nlm/internal/notebooklm/api"
)

type labelReader interface {
	GetLabels(ctx context.Context, projectID string) ([]api.Label, error)
}

type labelAttacher interface {
	AttachLabelSource(ctx context.Context, projectID, labelID, sourceID string) error
}

// labelsForSource returns the IDs of labels currently attached to sourceID.
// An empty result (with no error) means the source has no label assignments.
func labelsForSource(ctx context.Context, c labelReader, notebookID, sourceID string) ([]string, error) {
	labels, err := c.GetLabels(ctx, notebookID)
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

func attachLabelsToSources(ctx context.Context, c labelAttacher, notebookID string, sourceIDs, labelIDs []string) error {
	var failed []string
	for _, sid := range sourceIDs {
		for _, lid := range labelIDs {
			if err := c.AttachLabelSource(ctx, notebookID, lid, sid); err != nil {
				failed = append(failed, fmt.Sprintf("%s/%s: %v", lid, sid, err))
			}
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("attach %d/%d label assignments: %v", len(failed), len(sourceIDs)*len(labelIDs), failed)
	}
	return nil
}
