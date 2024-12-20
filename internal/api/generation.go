package api

import (
	"fmt"

	"github.com/tmc/nlm/internal/rpc"
)

type GenerationResponse struct {
	Content string
}

func (c *Client) GenerateNotebookGuide(projectID string) (*GenerationResponse, error) {
	_, err := c.be.Do(rpc.Call{
		ID:         rpc.RPCGenerateNotebookGuide,
		Args:       []interface{}{projectID},
		NotebookID: projectID,
	})
	if err != nil {
		return nil, fmt.Errorf("generate guide: %w", err)
	}
	return &GenerationResponse{}, nil
}

func (c *Client) GenerateOutline(projectID string) (*GenerationResponse, error) {
	_, err := c.be.Do(rpc.Call{
		ID:         rpc.RPCGenerateOutline,
		Args:       []interface{}{projectID},
		NotebookID: projectID,
	})
	if err != nil {
		return nil, fmt.Errorf("generate outline: %w", err)
	}
	return &GenerationResponse{}, nil
}

func (c *Client) GenerateSection(projectID string) (*GenerationResponse, error) {
	_, err := c.be.Do(rpc.Call{
		ID:         rpc.RPCGenerateSection,
		Args:       []interface{}{projectID},
		NotebookID: projectID,
	})
	if err != nil {
		return nil, fmt.Errorf("generate section: %w", err)
	}
	return &GenerationResponse{}, nil
}
