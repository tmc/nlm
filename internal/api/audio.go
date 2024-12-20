package api

import (
    "fmt"

    "github.com/tmc/nlm/internal/rpc"
)

func (c *Client) GetAudioOverview(projectID string) (*AudioOverview, error) {
    // Format: [["VUsiyb","[\"projectId\",0]",null,"generic"]]
    _ /* resp */, err := c.be.Do(rpc.Call{
        ID: rpc.RPCGetAudioOverview,
        Args: []interface{}{projectID, 0},
        NotebookID: projectID,
    })
    if err != nil {
        return nil, fmt.Errorf("get audio overview: %w", err)
    }
    // TODO: Parse response into AudioOverview
    return nil, nil
}

func (c *Client) CreateAudioOverview(projectID, instructions string) (*AudioOverview, error) {
    // Format: [["AHyHrd","[\"projectId\",\"instructions\"]",null,"generic"]]
    _ /* resp */, err := c.be.Do(rpc.Call{
        ID: rpc.RPCCreateAudioOverview,
        Args: []interface{}{projectID, instructions},
        NotebookID: projectID,
    })
    if err != nil {
        return nil, fmt.Errorf("create audio overview: %w", err)
    }
    // TODO: Parse response into AudioOverview
    return nil, nil
}

