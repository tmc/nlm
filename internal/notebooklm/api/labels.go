package api

import (
	"encoding/json"
	"fmt"

	"github.com/tmc/nlm/internal/notebooklm/rpc"
)

// Label is one autolabel cluster returned by GetLabels (I3xc3c). The wire
// shape per NotebookLM web UI captures (2026-04-23)
// is [name, [[src_id], ...], label_id, ""]; the trailing reserved string is
// always empty in observed traffic.
type Label struct {
	LabelID   string
	Name      string
	SourceIDs []string
}

// GetLabels returns the per-notebook autolabel clusters. An empty slice
// (rather than an error) means the user has not yet computed labels for
// this notebook.
//
// Wire request: [[2], project_id]. The leading [2] is a view enum required
// by the server — single-arg forms are rejected.
func (c *Client) GetLabels(projectID string) ([]Label, error) {
	if projectID == "" {
		return nil, fmt.Errorf("project ID required")
	}
	resp, err := c.rpc.Do(rpc.Call{
		ID:         rpc.RPCGetLabels,
		NotebookID: projectID,
		Args:       []interface{}{[]interface{}{2}, projectID},
	})
	if err != nil {
		return nil, fmt.Errorf("get labels: %w", err)
	}
	return parseLabelsResponse(resp)
}

// CreateLabel creates a new manual label and returns the refreshed label
// list. Emoji may be empty.
//
// Wire request: [[2], project_id, null, null, null, [[name, emoji]]].
// Response: [null, [[label-row, ...]]].
func (c *Client) CreateLabel(projectID, name, emoji string) ([]Label, error) {
	if projectID == "" {
		return nil, fmt.Errorf("project ID required")
	}
	if name == "" {
		return nil, fmt.Errorf("label name required")
	}
	resp, err := c.rpc.Do(rpc.Call{
		ID:         rpc.RPCMutateLabels,
		NotebookID: projectID,
		Args: []interface{}{
			[]interface{}{2}, projectID, nil, nil, nil,
			[]interface{}{[]interface{}{name, emoji}},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("create label: %w", err)
	}
	return parseLabelsResponse(resp)
}

// LabelUnlabeled assigns existing labels to sources that don't yet belong
// to one (mode 0) — what the UI fires after adding a label or new sources,
// without recomputing the cluster set. Returns the refreshed list.
//
// Wire request: [[2], project_id, null, null, [0]].
func (c *Client) LabelUnlabeled(projectID string) ([]Label, error) {
	return c.mutateLabelsMode(projectID, 0)
}

// RelabelAll triggers a full re-cluster (mode 1) — the modern UI's "Relabel
// all" button. On large notebooks this can hit the 60s server deadline and
// return DeadlineExceeded.
//
// Wire request: [[2], project_id, null, null, [1]].
func (c *Client) RelabelAll(projectID string) ([]Label, error) {
	return c.mutateLabelsMode(projectID, 1)
}

// GenerateLabels is a legacy alias for the empty-mode autolabel-recompute
// trigger. Behaviour appears equivalent to RelabelAll on observed traffic;
// new callers should prefer RelabelAll.
//
// Wire request: [[2], project_id, null, null, []].
func (c *Client) GenerateLabels(projectID string) ([]Label, error) {
	if projectID == "" {
		return nil, fmt.Errorf("project ID required")
	}
	resp, err := c.rpc.Do(rpc.Call{
		ID:         rpc.RPCMutateLabels,
		NotebookID: projectID,
		Args:       []interface{}{[]interface{}{2}, projectID, nil, nil, []interface{}{}},
	})
	if err != nil {
		return nil, fmt.Errorf("generate labels: %w", err)
	}
	return parseLabelsResponse(resp)
}

func (c *Client) mutateLabelsMode(projectID string, mode int) ([]Label, error) {
	if projectID == "" {
		return nil, fmt.Errorf("project ID required")
	}
	resp, err := c.rpc.Do(rpc.Call{
		ID:         rpc.RPCMutateLabels,
		NotebookID: projectID,
		Args: []interface{}{
			[]interface{}{2}, projectID, nil, nil,
			[]interface{}{mode},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("mutate labels (mode %d): %w", mode, err)
	}
	return parseLabelsResponse(resp)
}

// RenameLabel sets a new display name on an existing label.
//
// Wire request: [[2], project_id, label_id, [[[name]]]].
func (c *Client) RenameLabel(projectID, labelID, name string) error {
	if name == "" {
		return fmt.Errorf("name required")
	}
	return c.mutateLabel(projectID, labelID, []interface{}{[]interface{}{name}})
}

// SetLabelEmoji sets (or clears, if emoji is empty) the emoji on an
// existing label. The wire form leaves the name slot null so the server
// keeps the existing name.
//
// Wire request: [[2], project_id, label_id, [[[null, emoji]]]].
func (c *Client) SetLabelEmoji(projectID, labelID, emoji string) error {
	return c.mutateLabel(projectID, labelID, []interface{}{[]interface{}{nil, emoji}})
}

// AttachLabelSource adds a source to a label without changing the label's
// name or emoji. The wire shape carries exactly one source ID per call:
// when the UI assigns one source to N labels, it fires N concurrent le8sX
// calls. HAR captures from 2026-04-26 show two parallel calls at the same
// timestamp differing only in label_id. Bulk
// or remove forms have not been observed.
//
// Wire request: [[2], project_id, label_id, [[null, [[source_id]]]]].
func (c *Client) AttachLabelSource(projectID, labelID, sourceID string) error {
	if sourceID == "" {
		return fmt.Errorf("source ID required")
	}
	return c.mutateLabel(projectID, labelID, []interface{}{
		nil,
		[]interface{}{[]interface{}{sourceID}},
	})
}

// mutateLabel calls le8sX with the inner mutation payload. The outer envelope
// is constant: [[2], project_id, label_id, [<inner>]]. The two observed
// shapes for <inner> are [[name, emoji]] (metadata) and [null, [[source_id]]]
// (source attach); both fit the single-element-list form passed here.
func (c *Client) mutateLabel(projectID, labelID string, inner []interface{}) error {
	if projectID == "" {
		return fmt.Errorf("project ID required")
	}
	if labelID == "" {
		return fmt.Errorf("label ID required")
	}
	_, err := c.rpc.Do(rpc.Call{
		ID:         rpc.RPCMutateLabel,
		NotebookID: projectID,
		Args: []interface{}{
			[]interface{}{2}, projectID, labelID,
			[]interface{}{inner},
		},
	})
	if err != nil {
		return fmt.Errorf("mutate label: %w", err)
	}
	return nil
}

// DeleteLabels removes one or more labels from a notebook by label ID.
// The server response is empty on success.
//
// Wire request: [[2], project_id, [label_id, ...]].
func (c *Client) DeleteLabels(projectID string, labelIDs []string) error {
	if projectID == "" {
		return fmt.Errorf("project ID required")
	}
	if len(labelIDs) == 0 {
		return fmt.Errorf("at least one label ID required")
	}
	ids := make([]interface{}, len(labelIDs))
	for i, id := range labelIDs {
		ids[i] = id
	}
	_, err := c.rpc.Do(rpc.Call{
		ID:         rpc.RPCDeleteLabels,
		NotebookID: projectID,
		Args:       []interface{}{[]interface{}{2}, projectID, ids},
	})
	if err != nil {
		return fmt.Errorf("delete labels: %w", err)
	}
	return nil
}

func parseLabelsResponse(resp []byte) ([]Label, error) {
	var data []interface{}
	if err := json.Unmarshal(resp, &data); err != nil {
		return nil, fmt.Errorf("parse labels response: %w", err)
	}
	// agX4Bc returns [null, [[row, ...]]]; the leading null is a status slot.
	// Unwrap it so the rest of this function only sees the row container.
	if len(data) >= 2 && data[0] == nil {
		if inner, ok := data[1].([]interface{}); ok {
			data = inner
		}
	}
	// The wire response may arrive either flat ([row, row, ...]) or wrapped
	// ([[row, row, ...]]). Detect a wrapper by checking whether the first
	// element is itself a list-of-rows (i.e. its first element is a list).
	items := data
	if outer, ok := interfaceSliceAt(data, 0); ok {
		if _, innerIsRow := interfaceSliceAt(outer, 0); innerIsRow {
			items = outer
		}
	}
	labels := make([]Label, 0, len(items))
	for _, item := range items {
		row, ok := item.([]interface{})
		if !ok {
			continue
		}
		l := Label{
			Name:    stringAt(row, 0),
			LabelID: stringAt(row, 2),
		}
		if srcs, ok := interfaceSliceAt(row, 1); ok {
			for _, s := range srcs {
				inner, ok := s.([]interface{})
				if !ok {
					continue
				}
				if id := stringAt(inner, 0); id != "" {
					l.SourceIDs = append(l.SourceIDs, id)
				}
			}
		}
		if l.LabelID == "" && l.Name == "" {
			continue
		}
		labels = append(labels, l)
	}
	return labels, nil
}
