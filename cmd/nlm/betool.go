package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/tmc/nlm/internal/batchexecute"
)

// betool is a hidden developer command that translates raw batchexecute
// network payloads into structured JSON and back. It performs no network I/O
// and needs no authentication: it is a pure codec over the wire protocol,
// useful for inspecting HARs, hand-crafting requests, and round-trip testing.
//
// Modes:
//
//	decode-request   raw "f.req=...&at=...&" body        -> JSON WireRequest
//	encode-request   JSON WireRequest                    -> raw form body
//	decode-response  raw ")]}'"-prefixed response body   -> JSON WireResponse
//	encode-response  JSON WireResponse                   -> raw response body
//
// Input is read from a file argument, or from stdin when the argument is "-"
// or absent. Output goes to stdout.
func runBetool(args []string) error {
	if len(args) == 0 {
		printBetoolUsage()
		return fmt.Errorf("betool: missing mode")
	}
	mode := args[0]
	rest := args[1:]

	switch mode {
	case "decode-request", "encode-request", "decode-response", "encode-response":
	case "help", "-h", "--help":
		printBetoolUsage()
		return nil
	default:
		printBetoolUsage()
		return fmt.Errorf("betool: unknown mode %q", mode)
	}

	if len(rest) > 1 {
		return fmt.Errorf("betool %s: expected at most one input file, got %d args", mode, len(rest))
	}
	input, err := readBetoolInput(rest)
	if err != nil {
		return err
	}

	switch mode {
	case "decode-request":
		return betoolDecodeRequest(input)
	case "encode-request":
		return betoolEncodeRequest(input)
	case "decode-response":
		return betoolDecodeResponse(input)
	case "encode-response":
		return betoolEncodeResponse(input)
	}
	return nil // unreachable
}

// readBetoolInput reads the payload from a file argument, or from stdin when
// the argument is absent or "-".
func readBetoolInput(args []string) ([]byte, error) {
	if len(args) == 0 || args[0] == "-" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, fmt.Errorf("read stdin: %w", err)
		}
		return data, nil
	}
	data, err := os.ReadFile(args[0])
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", args[0], err)
	}
	return data, nil
}

func betoolDecodeRequest(input []byte) error {
	req, err := batchexecute.DecodeRequest(string(input))
	if err != nil {
		return fmt.Errorf("decode request: %w", err)
	}
	return writeJSON(req)
}

func betoolEncodeRequest(input []byte) error {
	var req batchexecute.WireRequest
	if err := json.Unmarshal(input, &req); err != nil {
		return fmt.Errorf("parse request JSON: %w", err)
	}
	body, err := batchexecute.EncodeRequest(&req)
	if err != nil {
		return fmt.Errorf("encode request: %w", err)
	}
	// Raw form bodies have no trailing newline on the wire; add one for
	// terminal readability without disturbing the payload.
	fmt.Fprintln(os.Stdout, body)
	return nil
}

func betoolDecodeResponse(input []byte) error {
	resp, err := batchexecute.DecodeResponse(string(input))
	if err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return writeJSON(resp)
}

func betoolEncodeResponse(input []byte) error {
	var resp batchexecute.WireResponse
	if err := json.Unmarshal(input, &resp); err != nil {
		return fmt.Errorf("parse response JSON: %w", err)
	}
	raw, err := batchexecute.EncodeResponse(&resp)
	if err != nil {
		return fmt.Errorf("encode response: %w", err)
	}
	fmt.Fprintln(os.Stdout, raw)
	return nil
}

// writeJSON writes v as indented JSON to stdout without HTML-escaping, so that
// wire payloads containing <, >, and & are reproduced faithfully.
func writeJSON(v any) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return fmt.Errorf("encode JSON: %w", err)
	}
	_, err := os.Stdout.Write(buf.Bytes())
	return err
}

func printBetoolUsage() {
	fmt.Fprint(os.Stderr, strings.TrimLeft(`
usage: nlm betool <mode> [file]

Translate raw batchexecute network payloads to JSON and back. Reads from
[file], or from stdin when [file] is "-" or omitted. Performs no network I/O.

Modes:
  decode-request    raw "f.req=...&at=...&" body      -> JSON
  encode-request    JSON request spec                 -> raw form body
  decode-response   raw ")]}'"-prefixed response body -> JSON
  encode-response   JSON response spec                -> raw response body

Examples:
  # Inspect a request captured from a HAR:
  pbpaste | nlm betool decode-request

  # Round-trip a response body:
  nlm betool decode-response resp.txt | nlm betool encode-response

  # Hand-craft a request body from JSON:
  echo '{"rpcs":[{"id":"wXbhsf","args":[]}],"at":"TOKEN"}' \
    | nlm betool encode-request
`, "\n"))
}
