package api

import (
    "fmt"

    "github.com/tmc/nlm/internal/rpc"
)

func (c *Client) ListRecentlyViewedProjects() ([]*Project, error) {
    // Format: [["wXbhsf","[null,1]",null,"generic"]]
    _ /* resp */, err := c.be.Do(rpc.Call{
        ID: rpc.RPCListRecentlyViewedProjects,
        Args: []interface{}{nil, 1},
    })
    if err != nil {
        return nil, fmt.Errorf("list projects: %w", err)
    }
    // TODO: Parse response into []*Project
    return nil, nil
}

func (c *Client) GetProject(id string) (*Project, error) {
    // Format: [["rLM1Ne","[\"projectId\"]",null,"generic"]]
    _ /* resp */, err := c.be.Do(rpc.Call{
        ID: rpc.RPCGetProject,
        Args: []interface{}{id},
        NotebookID: id,
    })
    if err != nil {
        return nil, fmt.Errorf("get project: %w", err)
    }
    // TODO: Parse response into *Project
    return nil, nil
}

