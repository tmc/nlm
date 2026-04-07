# Implementation Plan: Add --format flag to generate-chat

> **For Claude:** REQUIRED SUB-SKILL: Invoke `Read("/Users/vwh7mb/.claude/plugins/cache/edwinhu-plugins/workflows/2.43.0/lib/skills/dev-implement/SKILL.md")` to implement this plan.

## Approach

Fix root cause (gRPC path returns raw bytes) + add `--format` global flag for explicit clean-output mode.

## Requirements Coverage

| Req ID | Task |
|--------|------|
| FMT-01 | Task 3: Add --format flag in main.go |
| FMT-02 | Task 2: Fix gRPC path + Task 3: buffer/print |
| FMT-03 | Task 1+2: Parse through batchexecute stack |
| FMT-04 | Existing: errors already propagated via error return |

## Tasks

### Task 1: Add `DecodeBodyData` export to batchexecute package

**File:** `internal/batchexecute/batchexecute.go`

Add after line 58 (after the `Do` method):
```go
// DecodeBodyData parses a raw batchexecute HTTP response body and returns the
// data payload of the first wrb.fr response. This is used to parse gRPC-style
// endpoint responses that share the same chunked wire format.
func DecodeBodyData(body []byte) (json.RawMessage, error) {
    responses, err := decodeResponse(string(body))
    if err != nil {
        return nil, err
    }
    if len(responses) == 0 {
        return nil, fmt.Errorf("no responses found in body")
    }
    return responses[0].Data, nil
}
```

**Test:** Build passes with `go build ./...`

---

### Task 2: Fix gRPC path in `internal/api/client.go`

**File:** `internal/api/client.go`

**Changes:**
1. Add import: `"github.com/tmc/nlm/internal/beprotojson"`
2. Replace lines 1965-1975 (the gRPC success path that sets `Chunk: string(respBytes)`) with proper parsing

**New gRPC success path:**
```go
} else {
    if c.config.Debug {
        fmt.Fprintf(os.Stderr, "DEBUG:gRPC response: %s\n", string(respBytes))
    }
    // Parse the gRPC response — same chunked wire format as batchexecute
    data, parseErr := batchexecute.DecodeBodyData(respBytes)
    if parseErr == nil {
        var parsed pb.GenerateFreeFormStreamedResponse
        if unmarshalErr := beprotojson.Unmarshal(data, &parsed); unmarshalErr == nil {
            return &parsed, nil
        }
    }
    if c.config.Debug {
        fmt.Fprintf(os.Stderr, "DEBUG:gRPC response parse failed, trying batchexecute fallback\n")
    }
    // Fall through to batchexecute
}
```

**Test:** `go build ./...` passes; gRPC path now returns clean text

---

### Task 3: Add `--format` flag in `cmd/nlm/main.go`

**File:** `cmd/nlm/main.go`

**Changes:**

1. Add global var in the `var (...)` block:
   ```go
   outputFormat string // Output format: "stream" (default) or "plain"
   ```

2. In `init()`, add:
   ```go
   flag.StringVar(&outputFormat, "format", "stream", "output format: stream or plain (plain omits progress messages)")
   ```

3. Modify `generateFreeFormChat` signature + body:
   ```go
   func generateFreeFormChat(c *api.Client, projectID, prompt, format string) error {
       if format != "plain" {
           fmt.Fprintf(os.Stderr, "Generating response for: %s\n", prompt)
       }
       response, err := c.GenerateFreeFormStreamed(projectID, prompt, nil)
       if err != nil {
           return fmt.Errorf("generate chat: %w", err)
       }
       if response != nil && response.Chunk != "" {
           fmt.Println(response.Chunk)
       } else if format != "plain" {
           fmt.Println("(No response received)")
       }
       return nil
   }
   ```

4. Update call site (line ~802):
   ```go
   case "generate-chat":
       err = generateFreeFormChat(client, args[0], args[1], outputFormat)
   ```

**Test:** `nlm --format plain generate-chat <id> 'q'` outputs only answer text on stdout

---

## Testing Strategy

- **Framework:** `go build ./...` for compilation; `rsc.io/script` testdata for integration
- **Test command:** `go test ./cmd/nlm/...`
- **Existing test infra:** `cmd/nlm/testdata/*.txt` scripts

### Test for --format plain (add to testdata/generate_chat.txt or new file)

```
# Test that --format plain omits progress messages
exec nlm --format plain generate-chat $NOTEBOOK_ID 'test question'
! stdout 'Generating response for'  # progress goes to stderr, not stdout
stdout .                             # something on stdout
```

## Validation Criteria

- [ ] [FMT-01] `nlm --format plain generate-chat <id> <q>` executes without error
- [ ] [FMT-02] Output is readable prose text
- [ ] [FMT-03] Output contains no `wrb.fr`, `)]}'`, JSON artifacts
- [ ] [FMT-04] Exit code 0 on success
- [ ] `go build ./...` passes
- [ ] `go test ./...` passes
