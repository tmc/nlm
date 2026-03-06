# Investigation: Fix All Generation Commands

**Date:** 2025-02-02
**Status:** RESOLVED

## Summary

Multiple generation commands were failing with either 400 Bad Request or Service Unavailable errors. All commands have been fixed by redirecting them to use the working `GenerateFreeFormStreamed` gRPC endpoint with action-specific prompts.

## Commands Fixed

### 400 Bad Request Errors

| Command | Original RPC | Fix |
|---------|-------------|-----|
| `generate-section` | `cSX5Zc` | Use GenerateFreeFormStreamed with section prompt |

### Service Unavailable Errors

| Command | Original RPC | Fix |
|---------|-------------|-----|
| `generate-magic` | `eUBXe` | Use GenerateFreeFormStreamed with magic view prompt |
| `summarize` | `UivNle` (ActOnSources) | Redirect ActOnSources to GenerateFreeFormStreamed |
| `study-guide` | `UivNle` (ActOnSources) | Redirect ActOnSources to GenerateFreeFormStreamed |
| `faq` | `UivNle` (ActOnSources) | Redirect ActOnSources to GenerateFreeFormStreamed |
| `briefing-doc` | `UivNle` (ActOnSources) | Redirect ActOnSources to GenerateFreeFormStreamed |

## Root Cause

NotebookLM deprecated several RPC endpoints:
- `cSX5Zc` (GenerateSection) - returns 400
- `eUBXe` (GenerateMagicView) - returns Service Unavailable
- `UivNle` (ActOnSources) - returns Service Unavailable

The web UI now uses the `GenerateFreeFormStreamed` endpoint for all content generation tasks, using different prompts to achieve different output types.

## Solution

### 1. GenerateSection (`internal/api/client.go`)

```go
func (c *Client) GenerateSection(projectID string) (*pb.GenerateSectionResponse, error) {
    sectionPrompt := `Generate a new section of content based on the sources...`
    resp, err := c.GenerateFreeFormStreamed(projectID, sectionPrompt, nil)
    // ...
}
```

### 2. GenerateMagicView (`internal/api/client.go`)

```go
func (c *Client) GenerateMagicView(projectID string, sourceIDs []string) (*pb.GenerateMagicViewResponse, error) {
    magicPrompt := `Create a "magic view" synthesis of the selected sources...`
    resp, err := c.GenerateFreeFormStreamed(projectID, magicPrompt, sourceIDs)
    // ...
}
```

### 3. ActOnSources (`internal/api/client.go`)

Replaced the deprecated RPC call with a prompt-based approach:

```go
func (c *Client) ActOnSources(projectID string, action string, sourceIDs []string) error {
    prompts := map[string]string{
        "summarize":    `Summarize the key points from the selected sources...`,
        "study_guide":  `Generate a comprehensive study guide...`,
        "faq":          `Generate a Frequently Asked Questions document...`,
        "briefing_doc": `Create a professional briefing document...`,
        // ... and 10 more actions
    }

    prompt := prompts[action]
    resp, err := c.GenerateFreeFormStreamed(projectID, prompt, sourceIDs)
    fmt.Println(resp.Chunk)
    return nil
}
```

## Verification

All commands tested successfully:

```bash
./nlm generate-section <id>     # WORKS - generates content
./nlm generate-magic <id> <src> # WORKS - generates content
./nlm summarize <id> <src>      # WORKS - generates content
./nlm study-guide <id> <src>    # WORKS - generates content
./nlm faq <id> <src>            # WORKS - generates content
./nlm briefing-doc <id> <src>   # WORKS - generates content
```

## Files Changed

- `internal/api/client.go`:
  - `GenerateSection()` - now uses GenerateFreeFormStreamed
  - `GenerateMagicView()` - now uses GenerateFreeFormStreamed
  - `ActOnSources()` - now uses GenerateFreeFormStreamed with action-specific prompts

## Related

- `docs/investigations/2025-02-01_generate-chat-400-error.md` - Original gRPC fix
- `docs/investigations/2025-02-02_generate-outline-400-error.md` - Outline fix pattern
