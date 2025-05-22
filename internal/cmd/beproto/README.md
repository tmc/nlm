# BeProto

A utility for marshaling and unmarshaling between Protocol Buffers and NDJSON (Newline Delimited JSON) formats.

## Building

```sh
# From the project root
make beproto

# Or directly with Go
go build -o beproto ./internal/cmd/beproto
```

## Usage

```
BeProto - Utility for marshaling/unmarshaling between Protocol Buffers and NDJSON

Usage:
  beproto [flags]

Flags:
  -mode string    Mode: 'marshal' or 'unmarshal' (default "unmarshal")
  -help           Show this help message
  -debug          Enable debug output

Examples:
  # Unmarshal Protocol Buffer data to NDJSON
  cat data.proto | beproto -mode unmarshal > data.ndjson

  # Marshal NDJSON data to Protocol Buffer
  cat data.ndjson | beproto -mode marshal > data.proto
```

## Examples

### Unmarshal a Protocol Buffer response from NotebookLM API

```sh
# Save a raw API response to a file
nlm -debug ls 2>&1 | grep "Raw API response" | sed 's/.*Raw API response: //' > response.proto

# Convert it to readable JSON
./beproto -mode unmarshal < response.proto > response.json
```

### Create a test Protocol Buffer message from JSON

```sh
# Create a JSON file
echo '{"project_id":"test-123","title":"Test Notebook","emoji":"📘"}' > test.json

# Convert it to Protocol Buffer format
./beproto -mode marshal < test.json > test.proto
```

## Debugging

Use the `-debug` flag to see more information about the processing:

```sh
./beproto -mode unmarshal -debug < response.proto
```

This will show details about the input data size, parsing progress, and output size.