// Package api provides the NotebookLM API client.
package api

import (
	"fmt"

	pb "github.com/tmc/nlm/gen/notebooklm/v1alpha1"
	"github.com/tmc/nlm/internal/batchexecute"
	"github.com/tmc/nlm/internal/beprotojson"
	"github.com/tmc/nlm/internal/rpc"
)

type Notebook = pb.Project

// Client handles NotebookLM API interactions.
type Client struct {
	rpc *rpc.Client
}

// New creates a new NotebookLM API client.
func New(authToken, cookies string, opts ...batchexecute.Option) *Client {
	return &Client{
		rpc: rpc.New(authToken, cookies, opts...),
	}
}

func (c *Client) ListNotebooks() ([]*Notebook, error) {
	resp, err := c.rpc.Do(rpc.Call{
		ID:   rpc.RPCListNotebooks,
		Args: []interface{}{nil, 1},
	})
	if err != nil {
		return nil, fmt.Errorf("list notebooks failed: %w", err)
	}

	// Parse the response into a Project message
	var response pb.ListRecentlyViewedProjectsResponse
	if err := beprotojson.Unmarshal(resp, &response); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	return response.Projects, nil
}

// CreateNotebook creates a new notebook with the given title.
func (c *Client) CreateNotebook(title string) (*Notebook, error) {
	resp, err := c.rpc.Do(rpc.Call{
		ID:   rpc.RPCCreateNotebook,
		Args: []interface{}{title, "📔"},
	})
	if err != nil {
		return nil, fmt.Errorf("rpc call: %w", err)
	}

	var project pb.Project
	if err := beprotojson.Unmarshal(resp, &project); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	return &project, nil
}

// DeleteNotebook deletes a notebook by ID.
func (c *Client) DeleteNotebook(id string) error {
	_, err := c.rpc.Do(rpc.Call{
		ID:         rpc.RPCSync,
		Args:       []interface{}{id},
		NotebookID: id,
	})
	if err != nil {
		return fmt.Errorf("rpc call: %w", err)
	}
	return nil
}
