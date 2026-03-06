# Investigation: generate-outline 400 Bad Request Error

**Date:** 2025-02-02
**Status:** RESOLVED

## Summary

The `generate-outline` command was failing with a 400 Bad Request error. Root cause was that the `lCjAd` RPC endpoint for `GenerateOutline` has been deprecated by NotebookLM.

## Root Cause

The original `GenerateOutline` implementation used the batchexecute RPC with ID `lCjAd` and format `[project_id]`. This endpoint no longer works and returns 400 errors.

NotebookLM's web UI has moved to a new "Report API" with RPC ID `R7cb6c` that uses a complex template-based format for generating documents. However, implementing the full Report API would require significant reverse-engineering effort.

## Solution

Instead of implementing the new Report API, we modified `GenerateOutline()` to use the already-working `GenerateFreeFormStreamed` endpoint (which was fixed in the previous investigation) with a prompt that asks for an outline.

This approach:
1. Reuses the working gRPC endpoint infrastructure
2. Produces high-quality outlines using NotebookLM's chat capability
3. Requires minimal code changes

## Code Changes

Updated `internal/api/client.go` `GenerateOutline()` function:

```go
func (c *Client) GenerateOutline(projectID string) (*pb.GenerateOutlineResponse, error) {
    // The lCjAd RPC endpoint for GenerateOutline was deprecated.
    // Use GenerateFreeFormStreamed with an outline prompt instead.
    outlinePrompt := `Generate a comprehensive outline of all the content in this notebook...`

    // Use GenerateFreeFormStreamed which now works with the correct gRPC format
    resp, err := c.GenerateFreeFormStreamed(projectID, outlinePrompt, nil)
    if err != nil {
        return nil, fmt.Errorf("generate outline: %w", err)
    }

    // Convert the chat response to outline response format
    return &pb.GenerateOutlineResponse{
        Content: resp.Chunk,
    }, nil
}
```

## Verification

```bash
./nlm generate-outline d9c93c3f-c3e8-4f52-8fce-f828ddf6cf1a
# Returns: Comprehensive outline with sections like:
# - Part I: The Neuroscience of Motivation & Drive
# - I. The Role of Dopamine in Motivation
# - II. The Pleasure-Pain Balance
# - Part II: Understanding & Conquering Depression
# etc.
```

## Related

- Previous fix: `docs/investigations/2025-02-01_generate-chat-400-error.md`
- Both issues stem from NotebookLM API changes (batchexecute → gRPC-web)

## Future Considerations

1. The `generate-section` command likely has the same issue and can be fixed similarly
2. Other content transformation commands (`summarize`, `faq`, etc.) may need investigation
3. Response parsing could be improved to extract clean text from JSON streaming response
