package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/tmc/nlm/internal/batchexecute"
)

// decodeWrbFRRequest unwraps the form-encoded request used by Google's
// streaming endpoint. Its f.req value is [null,"<positional-json>",...].
func decodeWrbFRRequest(body []byte) (json.RawMessage, error) {
	form, err := url.ParseQuery(string(body))
	if err != nil {
		return nil, fmt.Errorf("parse stream request form: %w", err)
	}
	value := form.Get("f.req")
	if value == "" {
		return nil, fmt.Errorf("stream request has no f.req")
	}
	var envelope []json.RawMessage
	if err := json.Unmarshal([]byte(value), &envelope); err != nil {
		return nil, fmt.Errorf("parse stream request envelope: %w", err)
	}
	if len(envelope) < 2 {
		return nil, fmt.Errorf("stream request envelope has %d fields, want at least 2", len(envelope))
	}
	var payload string
	if err := json.Unmarshal(envelope[1], &payload); err != nil {
		return nil, fmt.Errorf("parse stream request payload: %w", err)
	}
	if !json.Valid([]byte(payload)) {
		return nil, fmt.Errorf("stream request payload is not valid JSON")
	}
	return json.RawMessage(payload), nil
}

// decodeWrbFRStream decodes Google's length-prefixed streaming response
// envelope. The wrb.fr payloads are cumulative snapshots, so the returned
// response contains only the final snapshot.
func decodeWrbFRStream(body []byte, rpcID string) (*batchexecute.WireResponse, int, error) {
	body = bytes.TrimSpace(bytes.TrimPrefix(body, []byte(")]}'")))
	scanner := bufio.NewScanner(bytes.NewReader(body))
	scanner.Buffer(make([]byte, 64*1024), 64*1024*1024)

	var final json.RawMessage
	chunks := 0
	frames := 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		declared, err := strconv.Atoi(line)
		if err != nil {
			return nil, frames, fmt.Errorf("stream chunk %d: parse length %q: %w", chunks+1, line, err)
		}
		if !scanner.Scan() {
			return nil, frames, fmt.Errorf("stream chunk %d: missing frame", chunks+1)
		}
		frame := strings.TrimSuffix(scanner.Text(), "\r")
		chunks++
		// Google's count includes the framing newlines and, despite being
		// described as a byte count, may count Unicode code points or UTF-16
		// code units when a frame contains non-ASCII text.
		utf16Len := len(utf16.Encode([]rune(frame)))
		if declared != len(frame)+2 && declared != utf8.RuneCountInString(frame)+2 && declared != utf16Len+2 {
			return nil, frames, fmt.Errorf("stream chunk %d: length %d does not match frame size %d", chunks, declared, len(frame))
		}
		var rows [][]json.RawMessage
		if err := json.Unmarshal([]byte(frame), &rows); err != nil {
			return nil, frames, fmt.Errorf("stream chunk %d: parse frame: %w", chunks, err)
		}
		for _, row := range rows {
			if len(row) < 3 {
				continue
			}
			var kind string
			if json.Unmarshal(row[0], &kind) != nil || kind != "wrb.fr" {
				continue
			}
			var payload string
			if err := json.Unmarshal(row[2], &payload); err != nil || payload == "" {
				continue
			}
			if !json.Valid([]byte(payload)) {
				return nil, frames, fmt.Errorf("stream chunk %d: wrb.fr payload is not valid JSON", chunks)
			}
			final = append(final[:0], payload...)
			frames++
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, frames, fmt.Errorf("scan stream: %w", err)
	}
	if frames == 0 {
		return nil, 0, fmt.Errorf("stream contains no wrb.fr payloads")
	}
	return &batchexecute.WireResponse{Responses: []batchexecute.WireRPCResponse{{
		ID: rpcID, Data: final,
	}}}, frames, nil
}
