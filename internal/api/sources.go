package api

import (
    "fmt"

    "github.com/tmc/nlm/internal/rpc"
)

func (c *Client) LoadSource(sourceID string) (*Source, error) {
    // Format: [["hizoJc","[[\"sourceId\"],[2]]",null,"generic"]]
    _ /* resp */, err := c.be.Do(rpc.Call{
        ID: rpc.RPCLoadSource,
        Args: []interface{}{
            []string{sourceID},
            []int{2},
        },
    })
    if err != nil {
        return nil, fmt.Errorf("load source: %w", err)
    }
    // TODO: Parse response into Source
    return nil, nil
}

// Helper functions for ActOnSources actions
func (c *Client) GenerateStudyGuide(projectID string, sourceIDs []string) (*Source, error) {
    return c.ActOnSources(projectID, sourceIDs, "notebook_guide_study_guide")
}

func (c *Client) GenerateFAQ(projectID string, sourceIDs []string) (*Source, error) {
    return c.ActOnSources(projectID, sourceIDs, "faq")
}

func (c *Client) GenerateTimeline(projectID string, sourceIDs []string) (*Source, error) {
    return c.ActOnSources(projectID, sourceIDs, "timeline")
}

func (c *Client) GenerateBriefingDoc(projectID string, sourceIDs []string) (*Source, error) {
    return c.ActOnSources(projectID, sourceIDs, "briefing_doc")
}
