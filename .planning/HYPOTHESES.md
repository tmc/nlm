# Debug Hypotheses

## Bug 3: --format plain does not clean output — still raw wrb.fr JSON
Started: 2026-04-06

### Symptom
- `nlm --format plain generate-chat <id> <q>` is accepted (no unknown flag error)
- Output is still raw streaming JSON with wrb.fr payloads
- The flag suppresses nothing / does not clean the response text

### Context
The --format plain flag was designed to:
1. Suppress "Generating response for:" progress message (stderr)
2. The actual text cleaning was expected to come from the gRPC path fix

BUT: if response.Chunk still contains raw JSON (wrb.fr), then fmt.Println(response.Chunk)
outputs raw JSON regardless of the format flag. The format flag does NOT filter/clean
the Chunk content — it only affects stderr progress messages.

So the real question is: why is response.Chunk still raw JSON after the gRPC fix?

### Candidates
1. extractTextFromJSON fallback in client.go is returning the full raw JSON blob
   (the longest "string" in the wrb.fr payload may BE a JSON-encoded string, not the text)
2. The batchexecute fallback path is still being hit (gRPC still failing) and
   batchexecute returns raw chunked bytes via a different code path
3. The format flag is not connected to any output filtering — it only gates stderr,
   so the user's expectation (clean stdout) requires actual Chunk cleaning

## Iteration Log

### H1 (CONFIRMED): DecodeBodyData succeeds but beprotojson.Unmarshal fails, and
extractTextFromJSON returns raw JSON string instead of clean text.

**Evidence from code trace (internal/api/client.go lines 1960-1995):**

The gRPC path in GenerateFreeFormStreamed has THREE result paths:

Path A (lines 1976-1977): beprotojson.Unmarshal succeeds → return parsed response.
  - If Chunk is correctly extracted, this works.
  - If Chunk is empty (wrong field mapping), main.go prints nothing.

Path B (lines 1983-1986): beprotojson.Unmarshal fails, but DecodeBodyData succeeded →
  return extractTextFromJSON(data) as Chunk.
  - The issue: for a real streaming gRPC response, `data` (what DecodeBodyData returns)
    is the JSON payload from wrb.fr position [2]. This is a JSON array like:
    `["text chunk here", false]` or a more complex nested structure.
  - extractTextFromJSON calls longestStringIn, which finds the longest string in the
    entire nested structure. If a session UUID, base64 blob, or the JSON-encoded
    inner payload string itself is longer than the actual answer text, it returns THAT
    instead of the human-readable text.

Path C (lines 1992-1994): DecodeBodyData fails (parseErr != nil) →
  return string(respBytes) as Chunk.
  - string(respBytes) = the entire raw HTTP response body, including the `)]}'` prefix,
    chunk-length numbers, and wrb.fr JSON arrays.
  - THIS is what produces the "raw streaming JSON with wrb.fr payloads" the user sees.
  - This is the CONFIRMED root cause path.

**Why does DecodeBodyData fail?**
The grpcendpoint.Execute sends with rt=c (chunked format). The streaming endpoint
GenerateFreeFormStreamed returns MULTIPLE wrb.fr entries (one per text chunk).
parseChunkedResponse's chunk-size tracking may miscount if server chunk sizes include
the trailing newline byte (off-by-one), causing the parser to consume the next chunk's
size line as part of the current chunk's data, corrupting subsequent parsing, and
ultimately returning "no valid responses found" → error → Path C → raw respBytes.

**The fix (two parts):**
1. Path C: Instead of returning string(respBytes), apply extractTextFromJSON to the
   raw bytes — or better, try to extract text by scanning respBytes for wrb.fr content.
   The safest fix: use a dedicated scan that finds all wrb.fr JSON strings and
   concatenates the text from each one.
2. Path B: The current extractTextFromJSON (longestStringIn) heuristic is fragile.
   For a GenerateFreeFormStreamed response, field 1 (position 0 in the array) IS the
   text chunk. A targeted extractor that reads position [0] from the decoded array
   would be more reliable than the longest-string heuristic.
