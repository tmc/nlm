package api

import (
    "fmt"
    "time"

    "github.com/tmc/nlm/internal/rpc"
)

func (c *Client) GetNotes(projectID string) ([]*Source, error) {
    // Format: [["cFji9","[\"projectId\",null,[timestamp,nanos]]",null,"generic"]]
    now := time.Now()
    unix := now.Unix()
    nanos := now.Nanosecond() / 1000 // Convert to microseconds as seen in requests

    _ /* resp */, err := c.be.Do(rpc.Call{
        ID: rpc.RPCGetNotes,
        Args: []interface{}{
            projectID,
            nil,
            []int64{unix, int64(nanos)},
        },
        NotebookID: projectID,
    })
    if err != nil {
        return nil, fmt.Errorf("get notes: %w", err)
    }
    // TODO: Parse response into []*Source
    return nil, nil
}

func (c *Client) ActOnSources(projectID string, sourceIDs []string, action string) (*Source, error) {
    // Format: [["yyryJe","[[[[\"sourceId\"]],null,null,null,null,[\"action\",[[[\"[CONTEXT]\",\"\"]],\"\"]]",null,"generic"]]
    sourceGroups := make([][1]string, len(sourceIDs))
    for i, id := range sourceIDs {
        sourceGroups[i] = [1]string{id}
    }

    _ /* resp */, err := c.be.Do(rpc.Call{
        ID: rpc.RPCActOnSources,
        Args: []interface{}{
            []interface{}{
                sourceGroups,
                nil, nil, nil, nil,
                []interface{}{
                    action,
                    [][]string{{"[CONTEXT]", ""}},
                    "",
                },
            },
        },
        NotebookID: projectID,
    })
    if err != nil {
        return nil, fmt.Errorf("act on sources: %w", err)
    }
    // TODO: Parse response
    return nil, nil
}

