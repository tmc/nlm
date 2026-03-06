# Investigation: generate-chat 400 Bad Request Error

**Date:** 2025-02-01
**Status:** RESOLVED

## Summary

The `generate-chat` command was failing with a 400 Bad Request error. Root cause was that NotebookLM switched from using batchexecute RPC format to a direct gRPC-web endpoint with a different request structure.

## Root Cause

NotebookLM's web UI now uses a direct gRPC-web endpoint instead of batchexecute:

```
/_/LabsTailwindUi/data/google.internal.labs.tailwind.orchestration.v1.LabsTailwindOrchestrationService/GenerateFreeFormStreamed
```

The request format also changed significantly.

## Old (Broken) Format

The CLI was sending via batchexecute:
```json
[[["BD", "[[[sources]], prompt, project_id, [2]]", null, "generic"]]]
```

## New (Working) Format

The browser sends to the gRPC endpoint with body `f.req=[null, "<inner_json>"]` where inner_json is:

```json
[
  [[[source_id_1]], [[source_id_2]]],  // [0] Sources - each wrapped as [[id]]
  "prompt",                            // [1] User question
  [],                                  // [2] Chat history (empty for first message)
  [2, null, [1], [1]],                 // [3] Config options
  "session_uuid",                      // [4] Chat session ID
  null,                                // [5]
  null,                                // [6]
  "project_id",                        // [7] Notebook/project ID
  1                                    // [8] Flag
]
```

Key differences:
1. **Source IDs format**: Each source wrapped as `[[id]]`, not `[id]`
2. **Chat history**: Position [2] contains conversation context (previous Q&A pairs)
3. **Config options**: `[2, null, [1], [1]]` not just `[2]`
4. **Project ID position**: Moved from position 2 to position 7
5. **Additional fields**: Session UUID, null placeholders, and flag

## Fix Applied

1. Updated `internal/rpc/grpcendpoint/handler.go`:
   - Added `BuildChatRequestWithProject()` function with correct format
   - Fixed source ID wrapping to `[[id]]` format
   - Added session UUID generation
   - Added proper config options

2. Updated `internal/api/client.go`:
   - Changed to use `BuildChatRequestWithProject()` with project ID

3. Updated `gen/method/LabsTailwindOrchestrationService_GenerateFreeFormStreamed_encoder.go`:
   - Updated format documentation
   - Fixed source ID wrapping for batchexecute fallback

## Verification

```bash
./nlm generate-chat d9c93c3f-c3e8-4f52-8fce-f828ddf6cf1a "What is dopamine?"
# Returns: AI-generated response about dopamine with citations
```

## How This Was Debugged

1. Used Chrome MCP extension to capture browser network requests
2. Installed JavaScript interceptor to capture XHR request bodies
3. Decoded and analyzed the `f.req` parameter structure
4. Compared browser format with CLI format
5. Identified the endpoint change (batchexecute → direct gRPC)
6. Updated encoder to match browser format

## Future Improvements

1. Response parsing could be improved to extract just the text content from JSON
2. Chat history support could be added for multi-turn conversations
3. Consider adding better error messages when format changes are detected
