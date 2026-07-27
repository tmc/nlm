package api

import (
	"context"
	"fmt"

	genmethod "github.com/tmc/nlm/gen/method"
	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/notebooklm/rpc"
	"google.golang.org/protobuf/proto"
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
func (c *Client) GetLabels(ctx context.Context, projectID string) ([]Label, error) {
	if projectID == "" {
		return nil, fmt.Errorf("project ID required")
	}
	response, err := c.orchestrationService.GetLabels(ctx, &pb.GetLabelsRequest{
		Context:   &pb.RequestContext{Version: proto.Int32(2)},
		ProjectId: projectID,
	})
	if err != nil {
		return nil, fmt.Errorf("get labels: %w", err)
	}
	return labelsFromProtoResponse(response), nil
}

func labelsFromProtoResponse(response *pb.GetLabelsResponse) []Label {
	if response == nil {
		return nil
	}
	labels := make([]Label, 0, len(response.GetLabels()))
	for _, label := range response.GetLabels() {
		if label == nil {
			continue
		}
		item := Label{Name: label.GetName(), LabelID: label.GetLabelId()}
		for _, source := range label.GetSources() {
			if sourceID := source.GetSourceId(); sourceID != "" {
				item.SourceIDs = append(item.SourceIDs, sourceID)
			}
		}
		if item.LabelID == "" && item.Name == "" {
			continue
		}
		labels = append(labels, item)
	}
	return labels
}

// CreateLabel creates a new manual label and returns the refreshed label
// list. Emoji may be empty.
//
// Wire request: [[2], project_id, null, null, null, [[name, emoji]]].
// Response: [null, [[label-row, ...]]].
func (c *Client) CreateLabel(ctx context.Context, projectID, name, emoji string) ([]Label, error) {
	if projectID == "" {
		return nil, fmt.Errorf("project ID required")
	}
	if name == "" {
		return nil, fmt.Errorf("label name required")
	}
	resp, err := c.orchestrationService.CreateLabel(ctx, &pb.CreateLabelRequest{
		Context:   &pb.RequestContext{Version: proto.Int32(2)},
		ProjectId: projectID,
		Creation:  &pb.LabelCreationRequest{Label: &pb.LabelCreation{Name: name, Emoji: proto.String(emoji)}},
	})
	if err != nil {
		return nil, fmt.Errorf("create label: %w", err)
	}
	return labelsFromProtoResponse(&pb.GetLabelsResponse{Labels: resp.GetLabels()}), nil
}

// LabelUnlabeled assigns existing labels to sources that don't yet belong
// to one (mode 0) — what the UI fires after adding a label or new sources,
// without recomputing the cluster set. Returns the refreshed list.
//
// Wire request: [[2], project_id, null, null, [0]].
func (c *Client) LabelUnlabeled(ctx context.Context, projectID string) ([]Label, error) {
	return c.mutateLabelsMode(ctx, projectID, 0)
}

// RelabelAll triggers a full re-cluster (mode 1) — the modern UI's "Relabel
// all" button. On large notebooks this can hit the 60s server deadline and
// return DeadlineExceeded.
//
// Wire request: [[2], project_id, null, null, [1]].
func (c *Client) RelabelAll(ctx context.Context, projectID string) ([]Label, error) {
	return c.mutateLabelsMode(ctx, projectID, 1)
}

// GenerateLabels triggers the empty-mode autolabel recompute form.
//
// Wire request: [[2], project_id, null, null, []].
func (c *Client) GenerateLabels(ctx context.Context, projectID string) ([]Label, error) {
	if projectID == "" {
		return nil, fmt.Errorf("project ID required")
	}
	resp, err := c.orchestrationService.MutateLabelsMode(ctx, &pb.MutateLabelsModeRequest{
		Context:   &pb.RequestContext{Version: proto.Int32(2)},
		ProjectId: projectID,
		Mode:      &pb.LabelMode{},
	})
	if err != nil {
		return nil, fmt.Errorf("generate labels: %w", err)
	}
	return labelsFromProtoResponse(&pb.GetLabelsResponse{Labels: resp.GetLabels()}), nil
}

func (c *Client) mutateLabelsMode(ctx context.Context, projectID string, mode int) ([]Label, error) {
	if projectID == "" {
		return nil, fmt.Errorf("project ID required")
	}
	resp, err := c.orchestrationService.MutateLabelsMode(ctx, &pb.MutateLabelsModeRequest{
		Context:   &pb.RequestContext{Version: proto.Int32(2)},
		ProjectId: projectID,
		Mode:      &pb.LabelMode{Value: proto.Int32(int32(mode))},
	})
	if err != nil {
		return nil, fmt.Errorf("mutate labels (mode %d): %w", mode, err)
	}
	return labelsFromProtoResponse(&pb.GetLabelsResponse{Labels: resp.GetLabels()}), nil
}

// RenameLabel sets a new display name on an existing label.
//
// Wire request: [[2], project_id, label_id, [[[name]]]].
func (c *Client) RenameLabel(ctx context.Context, projectID, labelID, name string) error {
	if name == "" {
		return fmt.Errorf("name required")
	}
	return c.mutateLabelProto(ctx, &pb.MutateLabelRequest{
		Context:   &pb.RequestContext{Version: proto.Int32(2)},
		ProjectId: projectID,
		LabelId:   labelID,
		Mutation:  &pb.MutateLabelMutation{Entry: &pb.MutateLabelEntry{Name: &pb.LabelNameChange{Name: proto.String(name)}}},
	})
}

// SetLabelEmoji sets (or clears, if emoji is empty) the emoji on an
// existing label. The wire form leaves the name slot null so the server
// keeps the existing name.
//
// Wire request: [[2], project_id, label_id, [[[null, emoji]]]].
func (c *Client) SetLabelEmoji(ctx context.Context, projectID, labelID, emoji string) error {
	return c.mutateLabel(ctx, projectID, labelID, []interface{}{[]interface{}{nil, emoji}})
}

// AttachLabelSource adds a source to a label without changing the label's
// name or emoji. The wire shape carries exactly one source ID per call:
// when the UI assigns one source to N labels, it fires N concurrent le8sX
// calls. HAR captures from 2026-04-26 show two parallel calls at the same
// timestamp differing only in label_id. Bulk
// or remove forms have not been observed.
//
// Wire request: [[2], project_id, label_id, [[null, [[source_id]]]]].
func (c *Client) AttachLabelSource(ctx context.Context, projectID, labelID, sourceID string) error {
	if sourceID == "" {
		return fmt.Errorf("source ID required")
	}
	return c.mutateLabelProto(ctx, &pb.MutateLabelRequest{
		Context:   &pb.RequestContext{Version: proto.Int32(2)},
		ProjectId: projectID,
		LabelId:   labelID,
		Mutation: &pb.MutateLabelMutation{Entry: &pb.MutateLabelEntry{
			Sources: []*pb.SourceIdList{{SourceId: sourceID}},
		}},
	})
}

func (c *Client) mutateLabelProto(ctx context.Context, req *pb.MutateLabelRequest) error {
	if req.GetProjectId() == "" {
		return fmt.Errorf("project ID required")
	}
	if req.GetLabelId() == "" {
		return fmt.Errorf("label ID required")
	}
	if _, err := c.rpc.Do(ctx, rpc.Call{
		ID:         rpc.RPCMutateLabel,
		NotebookID: req.GetProjectId(),
		Args:       genmethod.EncodeMutateLabelArgs(req),
	}); err != nil {
		return fmt.Errorf("mutate label: %w", err)
	}
	return nil
}

// mutateLabel calls le8sX with the inner mutation payload. The outer envelope
// is constant: [[2], project_id, label_id, [<inner>]]. The two observed
// shapes for <inner> are [[name, emoji]] (metadata) and [null, [[source_id]]]
// (source attach); both fit the single-element-list form passed here.
func (c *Client) mutateLabel(ctx context.Context, projectID, labelID string, inner []interface{}) error {
	if projectID == "" {
		return fmt.Errorf("project ID required")
	}
	if labelID == "" {
		return fmt.Errorf("label ID required")
	}
	_, err := c.rpc.Do(ctx, rpc.Call{
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
func (c *Client) DeleteLabels(ctx context.Context, projectID string, labelIDs []string) error {
	if projectID == "" {
		return fmt.Errorf("project ID required")
	}
	if len(labelIDs) == 0 {
		return fmt.Errorf("at least one label ID required")
	}
	_, err := c.orchestrationService.DeleteLabels(ctx, &pb.DeleteLabelsRequest{
		ProjectId: projectID,
		LabelIds:  labelIDs,
	})
	if err != nil {
		return fmt.Errorf("delete labels: %w", err)
	}
	return nil
}
