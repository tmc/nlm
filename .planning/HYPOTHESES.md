# Debug Hypotheses

## Bug: generate-chat returns 400 Bad Request on every call
Started: 2026-04-06
Status: FIXED

## Root Cause (Confirmed)

The recent refactor of `GenerateFreeFormStreamed` in `internal/api/client.go` broke
the gRPC response handling. Specifically:

**Before (working):** When the gRPC endpoint call succeeded, the code immediately
returned a synthetic `GenerateFreeFormStreamedResponse{Chunk: string(respBytes)}`,
always giving the caller a result.

**After (broken):** The refactor added `DecodeBodyData + beprotojson.Unmarshal` to
parse the gRPC response. When either step fails (which happens because the gRPC
streaming response format doesn't match `GenerateFreeFormStreamedResponse`'s
`[chunk_text, is_final]` schema), the code falls through to the batchexecute BD RPC.

The BD batchexecute RPC is **deprecated** (documented in
`docs/investigations/2025-02-01_generate-chat-400-error.md` — the February 2025 fix
specifically replaced it with the gRPC endpoint). The server returns 400 for it.

## Fix Applied

**File:** `internal/api/client.go`

When the gRPC call succeeds (no network/HTTP error), we now never fall through to
batchexecute. Instead:
1. If `DecodeBodyData + beprotojson.Unmarshal` both succeed → return parsed response
2. If unmarshal fails but decode succeeded → extract longest string from JSON data
   via new `extractTextFromJSON` helper, return as chunk
3. If decode also fails → return raw response bytes as chunk

The batchexecute fallback is now only reachable when the gRPC endpoint itself fails
(network error or HTTP error status), which is appropriate.

**Helper added:** `extractTextFromJSON(json.RawMessage) string` + `longestStringIn`
recursive helper, both in `internal/api/client.go`.

**Regression test added:** `TestExtractTextFromJSON` in `internal/api/client_test.go`

## Hypothesis Log
| # | Hypothesis | Test | Result |
|---|-----------|------|--------|
| H1 | Missing NotebookID in batchexecute Call causes source-path="/" which 400s | Checked both GenerateFreeFormStreamed and GenerateNotebookGuide service clients — neither sets NotebookID. But generate-guide works, so missing NotebookID is not the cause. | REJECTED |
| H2 | gRPC parse fails silently, falls through to deprecated BD batchexecute RPC | Traced code path: gRPC succeeds, DecodeBodyData+Unmarshal fail, falls through to `c.orchestrationService.GenerateFreeFormStreamed` which uses BD RPC. Confirmed BD is deprecated per docs/investigations/2025-02-01_generate-chat-400-error.md | CONFIRMED |
