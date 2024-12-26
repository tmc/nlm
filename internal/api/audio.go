package api

import (
	"fmt"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/beprotojson"
	"github.com/tmc/nlm/internal/rpc"
)

func (c *Client) GetAudioOverview(projectID string) (*AudioOverview, error) {
	resp, err := c.be.Do(rpc.Call{
		ID:         rpc.RPCGetAudioOverview,
		Args:       []interface{}{projectID, 0},
		NotebookID: projectID,
	})
	if err != nil {
		return nil, fmt.Errorf("get audio overview: %w", err)
	}

	var result pb.AudioOverview
	if err := beprotojson.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	return &result, nil
}

func (c *Client) CreateAudioOverview(projectID, instructions string) (*AudioOverview, error) {
	resp, err := c.be.Do(rpc.Call{
		ID:         rpc.RPCCreateAudioOverview,
		Args:       []interface{}{projectID, instructions},
		NotebookID: projectID,
	})
	if err != nil {
		return nil, fmt.Errorf("create audio overview: %w", err)
	}

	var result pb.AudioOverview
	if err := beprotojson.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	return &result, nil
}
